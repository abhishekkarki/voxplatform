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
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	voxv1alpha1 "github.com/abhishekkarki/voxplatform/operator/api/v1alpha1"
)

// workflowGVK identifies Argo Workflow objects. The operator deliberately does
// not vendor the Argo Go SDK (a large transitive dependency tree) — it
// builds and reads Workflow objects as unstructured.Unstructured instead,
// the same way any client without a typed scheme entry for a foreign CRD
// would. This is the only place that GVK is needed.
var workflowGVK = schema.GroupVersionKind{
	Group:   "argoproj.io",
	Version: "v1alpha1",
	Kind:    "Workflow",
}

// evalReportOutputParam is the name of the Workflow output parameter the eval
// container's --report-path file is exposed as. Its value is the full JSON
// report produced by vox_eval.runner.EvalReport.to_dict() — this is the
// entire Go/Python contract for this feature; no other code is shared.
const evalReportOutputParam = "eval-report"

// evalReport mirrors the subset of EvalReport.to_dict() (eval/vox_eval/runner.py)
// the controller needs. Extra fields in the JSON are ignored.
type evalReport struct {
	Passed       bool    `json:"passed"`
	CorpusWER    float64 `json:"corpus_wer"`
	MeanWER      float64 `json:"mean_wer"`
	TotalSamples int32   `json:"total_samples"`
}

// EvalRunReconciler reconciles EvalRun objects.
//
// An EvalRun is one-shot, like a Job: on first reconcile it submits exactly
// one Argo Workflow (owned by the EvalRun, so it's garbage-collected with
// it) that runs the vox-eval harness, then on subsequent reconciles mirrors
// the Workflow's outcome into EvalRun status.
type EvalRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vox.vox.io,resources=evalruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vox.vox.io,resources=evalruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vox.vox.io,resources=evalruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete

func (r *EvalRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var evalRun voxv1alpha1.EvalRun
	if err := r.Get(ctx, req.NamespacedName, &evalRun); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching EvalRun: %w", err)
	}

	if !evalRun.DeletionTimestamp.IsZero() {
		logger.Info("EvalRun is being deleted, nothing to clean up (Workflow is owned)")
		return ctrl.Result{}, nil
	}

	if evalRun.Spec.Suspend {
		return r.setStatus(ctx, &evalRun, voxv1alpha1.EvalRunPhasePending, evalRun.Status.WorkflowRef, "suspended: no Workflow submitted")
	}

	if evalRun.Status.WorkflowRef == "" {
		return r.submitWorkflow(ctx, &evalRun)
	}

	return r.reconcileWorkflow(ctx, &evalRun)
}

// submitWorkflow builds and creates the Argo Workflow for a fresh EvalRun.
func (r *EvalRunReconciler) submitWorkflow(ctx context.Context, evalRun *voxv1alpha1.EvalRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wf := buildWorkflow(evalRun)
	if err := controllerutil.SetControllerReference(evalRun, wf, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting owner reference on Workflow: %w", err)
	}

	if err := r.Create(ctx, wf); err != nil && !errors.IsAlreadyExists(err) {
		return ctrl.Result{}, fmt.Errorf("creating Workflow: %w", err)
	}

	logger.Info("submitted eval Workflow", "workflow", wf.GetName())

	now := metav1.Now()
	evalRun.Status.StartTime = &now
	return r.setStatus(ctx, evalRun, voxv1alpha1.EvalRunPhaseRunning, wf.GetName(), "Workflow submitted")
}

// buildWorkflow constructs the Argo Workflow object for an EvalRun: a single
// container step running `vox-eval run`, with its JSON report exposed as a
// Workflow output parameter so the controller can read it back.
func buildWorkflow(evalRun *voxv1alpha1.EvalRun) *unstructured.Unstructured {
	threshold := evalRun.Spec.WERThreshold
	if threshold == "" {
		threshold = "0.3"
	}

	timeoutSeconds := int64(600)
	if evalRun.Spec.TimeoutSeconds != nil {
		timeoutSeconds = int64(*evalRun.Spec.TimeoutSeconds)
	}

	const reportPath = "/tmp/outputs/report.json"

	args := []interface{}{
		"run",
		evalRun.Spec.Dataset,
		"--url", evalRun.Spec.GatewayURL,
		"--threshold", threshold,
		"--report-path", reportPath,
	}
	if evalRun.Spec.Model != "" {
		args = append(args, "--model", evalRun.Spec.Model)
	}

	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(workflowGVK)
	wf.SetName(evalRun.Name)
	wf.SetNamespace(evalRun.Namespace)
	wf.SetLabels(map[string]string{
		"vox.vox.io/evalrun": evalRun.Name,
	})

	wf.Object["spec"] = map[string]interface{}{
		"entrypoint":            "eval",
		"activeDeadlineSeconds": timeoutSeconds,
		"templates": []interface{}{
			map[string]interface{}{
				"name": "eval",
				"container": map[string]interface{}{
					"image":   evalRun.Spec.Image,
					"command": []interface{}{"vox-eval"},
					"args":    args,
				},
				"outputs": map[string]interface{}{
					"parameters": []interface{}{
						map[string]interface{}{
							"name": evalReportOutputParam,
							"valueFrom": map[string]interface{}{
								"path": reportPath,
							},
						},
					},
				},
			},
		},
	}

	return wf
}

// reconcileWorkflow fetches the previously-submitted Workflow and projects
// its outcome into EvalRun status.
func (r *EvalRunReconciler) reconcileWorkflow(ctx context.Context, evalRun *voxv1alpha1.EvalRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(workflowGVK)
	err := r.Get(ctx, types.NamespacedName{Name: evalRun.Status.WorkflowRef, Namespace: evalRun.Namespace}, wf)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.setStatus(ctx, evalRun, voxv1alpha1.EvalRunPhaseFailed, evalRun.Status.WorkflowRef, "Workflow no longer exists")
		}
		return ctrl.Result{}, fmt.Errorf("fetching Workflow: %w", err)
	}

	workflowPhase, _, _ := unstructured.NestedString(wf.Object, "status", "phase")

	if !isTerminalWorkflowPhase(workflowPhase) {
		return r.setStatus(ctx, evalRun, voxv1alpha1.EvalRunPhaseRunning, wf.GetName(), fmt.Sprintf("Workflow phase: %s", orPending(workflowPhase)))
	}

	report, found := extractEvalReport(wf)
	if !found {
		logger.Info("Workflow finished with no eval report", "workflow", wf.GetName(), "workflowPhase", workflowPhase)
		return r.setStatus(ctx, evalRun, voxv1alpha1.EvalRunPhaseFailed, wf.GetName(),
			fmt.Sprintf("Workflow %s with no eval report produced (crashed before writing it?)", workflowPhase))
	}

	phase := voxv1alpha1.EvalRunPhaseSucceeded
	message := fmt.Sprintf("corpus WER %.4f within threshold", report.CorpusWER)
	if !report.Passed {
		phase = voxv1alpha1.EvalRunPhaseFailed
		message = fmt.Sprintf("corpus WER %.4f exceeds threshold %s", report.CorpusWER, evalRun.Spec.WERThreshold)
	}

	evalRun.Status.CorpusWER = fmt.Sprintf("%.4f", report.CorpusWER)
	evalRun.Status.MeanWER = fmt.Sprintf("%.4f", report.MeanWER)
	evalRun.Status.TotalSamples = report.TotalSamples
	evalRun.Status.Passed = &report.Passed

	return r.setStatus(ctx, evalRun, phase, wf.GetName(), message)
}

// extractEvalReport reads the eval-report output parameter off a terminal
// Workflow and unmarshals it. Argo's wait sidecar captures declared output
// parameters after the main container exits, whether it exited 0 or not, so
// this is readable even when the Workflow itself is Failed (e.g. vox-eval
// exits 1 on a WER regression).
func extractEvalReport(wf *unstructured.Unstructured) (*evalReport, bool) {
	params, found, err := unstructured.NestedSlice(wf.Object, "status", "outputs", "parameters")
	if err != nil || !found {
		return nil, false
	}

	for _, p := range params {
		param, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := param["name"].(string); name != evalReportOutputParam {
			continue
		}
		value, ok := param["value"].(string)
		if !ok || value == "" {
			return nil, false
		}

		var report evalReport
		if err := json.Unmarshal([]byte(value), &report); err != nil {
			return nil, false
		}
		return &report, true
	}

	return nil, false
}

// isTerminalWorkflowPhase reports whether an Argo Workflow phase is one it
// will not transition out of on its own.
func isTerminalWorkflowPhase(phase string) bool {
	switch phase {
	case "Succeeded", "Failed", "Error":
		return true
	default:
		return false
	}
}

func orPending(phase string) string {
	if phase == "" {
		return "Pending"
	}
	return phase
}

func (r *EvalRunReconciler) setStatus(
	ctx context.Context,
	evalRun *voxv1alpha1.EvalRun,
	phase voxv1alpha1.EvalRunPhase,
	workflowRef string,
	message string,
) (ctrl.Result, error) {
	if evalRun.Status.Phase != phase && (phase == voxv1alpha1.EvalRunPhaseSucceeded || phase == voxv1alpha1.EvalRunPhaseFailed) {
		now := metav1.Now()
		evalRun.Status.CompletionTime = &now
	}

	evalRun.Status.Phase = phase
	evalRun.Status.WorkflowRef = workflowRef
	evalRun.Status.Message = message

	if err := r.Status().Update(ctx, evalRun); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating EvalRun status: %w", err)
	}

	if phase == voxv1alpha1.EvalRunPhaseRunning {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
// It owns the Argo Workflow it submits, so a Workflow status change (e.g.
// Argo marking it Succeeded/Failed) immediately requeues the owning EvalRun.
func (r *EvalRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(workflowGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(&voxv1alpha1.EvalRun{}).
		Owns(wf).
		Complete(r)
}
