/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	voxv1alpha1 "github.com/abhishekkarki/voxplatform/operator/api/v1alpha1"
)

// InferencePipelineReconciler reconciles InferencePipeline objects.
//
// The reconciler does not create any owned K8s resources. Its job is purely
// to validate that every referenced VoiceModel exists and is Ready, then
// project that readiness into the pipeline's own status. This makes the
// InferencePipeline a health-check aggregate: if it's Ready, the gateway
// can run all stages; if it's Degraded, some stages may be unavailable.
type InferencePipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vox.vox.io,resources=inferencepipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vox.vox.io,resources=inferencepipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vox.vox.io,resources=inferencepipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=vox.vox.io,resources=voicemodels,verbs=get;list;watch

func (r *InferencePipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pipeline voxv1alpha1.InferencePipeline
	if err := r.Get(ctx, req.NamespacedName, &pipeline); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching InferencePipeline: %w", err)
	}

	if !pipeline.DeletionTimestamp.IsZero() {
		logger.Info("InferencePipeline is being deleted, nothing to clean up")
		return ctrl.Result{}, nil
	}

	return r.reconcileStages(ctx, &pipeline)
}

// reconcileStages inspects every stage, resolves its VoiceModel, and
// computes the aggregate phase for the pipeline.
func (r *InferencePipelineReconciler) reconcileStages(
	ctx context.Context,
	pipeline *voxv1alpha1.InferencePipeline,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	stageStatuses := make([]voxv1alpha1.PipelineStageStatus, 0, len(pipeline.Spec.Stages))
	readyCount := 0
	enabledCount := 0
	hasMissing := false

	for _, stage := range pipeline.Spec.Stages {
		enabled := stage.Enabled == nil || *stage.Enabled
		if !enabled {
			stageStatuses = append(stageStatuses, voxv1alpha1.PipelineStageStatus{
				Name:     stage.Name,
				ModelRef: stage.Model,
				Ready:    false,
				Message:  "stage disabled",
			})
			continue
		}
		enabledCount++

		status := r.resolveStage(ctx, stage, pipeline.Namespace)
		stageStatuses = append(stageStatuses, status)

		if status.Ready {
			readyCount++
		}
		if status.Message == "VoiceModel not found" {
			hasMissing = true
		}
	}

	var phase voxv1alpha1.InferencePipelinePhase
	var message string

	switch {
	case hasMissing:
		phase = voxv1alpha1.PipelinePhaseFailed
		message = "one or more referenced VoiceModels do not exist"
	case enabledCount == 0:
		phase = voxv1alpha1.PipelinePhaseReady
		message = "no enabled stages"
	case readyCount == enabledCount:
		phase = voxv1alpha1.PipelinePhaseReady
		message = fmt.Sprintf("%d/%d stages ready", readyCount, enabledCount)
	case readyCount > 0:
		phase = voxv1alpha1.PipelinePhaseDegraded
		message = fmt.Sprintf("%d/%d stages ready", readyCount, enabledCount)
	default:
		phase = voxv1alpha1.PipelinePhaseValidating
		message = fmt.Sprintf("0/%d stages ready, waiting for VoiceModels", enabledCount)
	}

	logger.Info("pipeline evaluated", "phase", phase, "ready", readyCount, "enabled", enabledCount)

	return r.setPipelineStatus(ctx, pipeline, phase, message, stageStatuses)
}

// resolveStage looks up the VoiceModel for a single stage and builds its status.
func (r *InferencePipelineReconciler) resolveStage(
	ctx context.Context,
	stage voxv1alpha1.PipelineStage,
	namespace string,
) voxv1alpha1.PipelineStageStatus {
	status := voxv1alpha1.PipelineStageStatus{
		Name:     stage.Name,
		ModelRef: stage.Model,
	}

	var vm voxv1alpha1.VoiceModel
	err := r.Get(ctx, types.NamespacedName{Name: stage.Model, Namespace: namespace}, &vm)
	if err != nil {
		if errors.IsNotFound(err) {
			status.Message = "VoiceModel not found"
		} else {
			status.Message = fmt.Sprintf("error fetching VoiceModel: %v", err)
		}
		return status
	}

	if vm.Status.Phase == voxv1alpha1.PhaseReady {
		status.Ready = true
		status.Endpoint = vm.Status.Endpoint
		status.Message = "ready"
	} else {
		phase := string(vm.Status.Phase)
		if phase == "" {
			phase = "Pending"
		}
		status.Message = fmt.Sprintf("VoiceModel phase: %s", phase)
	}

	return status
}

func (r *InferencePipelineReconciler) setPipelineStatus(
	ctx context.Context,
	pipeline *voxv1alpha1.InferencePipeline,
	phase voxv1alpha1.InferencePipelinePhase,
	message string,
	stages []voxv1alpha1.PipelineStageStatus,
) (ctrl.Result, error) {
	now := metav1.Now()

	if pipeline.Status.Phase != phase {
		pipeline.Status.LastTransitionTime = &now
	}

	pipeline.Status.Phase = phase
	pipeline.Status.Message = message
	pipeline.Status.Stages = stages

	if err := r.Status().Update(ctx, pipeline); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating InferencePipeline status: %w", err)
	}

	if phase != voxv1alpha1.PipelinePhaseReady && phase != voxv1alpha1.PipelinePhaseFailed {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
// We watch both InferencePipeline and VoiceModel objects so that
// a VoiceModel transitioning to Ready immediately triggers a pipeline reconcile.
func (r *InferencePipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&voxv1alpha1.InferencePipeline{}).
		Watches(
			&voxv1alpha1.VoiceModel{},
			handler.EnqueueRequestsFromMapFunc(mapVoiceModelToPipelines(mgr.GetClient())),
		).
		Complete(r)
}

// mapVoiceModelToPipelines returns a MapFunc that, when a VoiceModel changes,
// lists all InferencePipelines in the same namespace and enqueues those that
// reference the changed VoiceModel. This ensures pipelines reconcile
// immediately when a backing model transitions to Ready.
func mapVoiceModelToPipelines(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		vm, ok := obj.(*voxv1alpha1.VoiceModel)
		if !ok {
			return nil
		}

		var pipelineList voxv1alpha1.InferencePipelineList
		if err := c.List(ctx, &pipelineList, client.InNamespace(vm.Namespace)); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, p := range pipelineList.Items {
			for _, stage := range p.Spec.Stages {
				if stage.Model == vm.Name {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      p.Name,
							Namespace: p.Namespace,
						},
					})
					break
				}
			}
		}

		return requests
	}
}
