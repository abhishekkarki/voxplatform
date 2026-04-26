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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	voxv1alpha1 "github.com/abhishekkarki/voxplatform/operator/api/v1alpha1"
)

const (
	finalizerName   = "vox.io/voicemodel-cleanup"
	defaultCPUImage = "fedirz/faster-whisper-server:0.3.0-cpu"
	defaultGPUImage = "fedirz/faster-whisper-server:0.3.0-cuda"
)

// VoiceModelReconciler reconciles VoiceModel objects.
type VoiceModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vox.vox.io,resources=voicemodels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vox.vox.io,resources=voicemodels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vox.vox.io,resources=voicemodels/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main entry point — called every time a VoiceModel
// changes or every time the requeue timer fires.
func (r *VoiceModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the VoiceModel
	var voiceModel voxv1alpha1.VoiceModel
	if err := r.Get(ctx, req.NamespacedName, &voiceModel); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("VoiceModel not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching VoiceModel: %w", err)
	}

	// 2. Handle deletion
	if !voiceModel.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &voiceModel)
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(&voiceModel, finalizerName) {
		controllerutil.AddFinalizer(&voiceModel, finalizerName)
		if err := r.Update(ctx, &voiceModel); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
	}

	// 3. Reconcile the Deployment
	if err := r.reconcileDeployment(ctx, &voiceModel); err != nil {
		return r.setPhase(ctx, &voiceModel, voxv1alpha1.PhaseFailed, err.Error())
	}

	// 4. Reconcile the Service
	if err := r.reconcileService(ctx, &voiceModel); err != nil {
		return r.setPhase(ctx, &voiceModel, voxv1alpha1.PhaseFailed, err.Error())
	}

	// 5. Update status based on Deployment state
	return r.updateStatus(ctx, &voiceModel)
}

func (r *VoiceModelReconciler) reconcileDelete(ctx context.Context, vm *voxv1alpha1.VoiceModel) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("cleaning up VoiceModel", "name", vm.Name)

	controllerutil.RemoveFinalizer(vm, finalizerName)
	if err := r.Update(ctx, vm); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *VoiceModelReconciler) reconcileDeployment(ctx context.Context, vm *voxv1alpha1.VoiceModel) error {
	logger := log.FromContext(ctx)
	deployName := deploymentName(vm)

	desired := r.buildDeployment(vm)

	if err := controllerutil.SetControllerReference(vm, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: vm.Namespace}, &existing)

	if errors.IsNotFound(err) {
		logger.Info("creating Deployment", "name", deployName)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("fetching Deployment: %w", err)
	}

	existing.Spec = desired.Spec
	logger.Info("updating Deployment", "name", deployName)
	return r.Update(ctx, &existing)
}

func (r *VoiceModelReconciler) reconcileService(ctx context.Context, vm *voxv1alpha1.VoiceModel) error {
	logger := log.FromContext(ctx)
	svcName := serviceName(vm)

	desired := r.buildService(vm)

	if err := controllerutil.SetControllerReference(vm, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	var existing corev1.Service
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: vm.Namespace}, &existing)

	if errors.IsNotFound(err) {
		logger.Info("creating Service", "name", svcName)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("fetching Service: %w", err)
	}

	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	existing.Spec = desired.Spec
	logger.Info("updating Service", "name", svcName)
	return r.Update(ctx, &existing)
}

func (r *VoiceModelReconciler) updateStatus(ctx context.Context, vm *voxv1alpha1.VoiceModel) (ctrl.Result, error) {
	deployName := deploymentName(vm)

	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: vm.Namespace}, &deploy); err != nil {
		return r.setPhase(ctx, vm, voxv1alpha1.PhaseDeploying, "waiting for Deployment")
	}

	vm.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	vm.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		serviceName(vm), vm.Namespace, getPort(vm))

	replicas := int32(1)
	if vm.Spec.Replicas != nil {
		replicas = *vm.Spec.Replicas
	}

	switch {
	case deploy.Status.ReadyReplicas >= replicas:
		return r.setPhase(ctx, vm, voxv1alpha1.PhaseReady,
			fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, replicas))
	case deploy.Status.ReadyReplicas > 0:
		return r.setPhase(ctx, vm, voxv1alpha1.PhaseDeploying,
			fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, replicas))
	default:
		for _, cond := range deploy.Status.Conditions {
			if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
				return r.setPhase(ctx, vm, voxv1alpha1.PhaseFailed, cond.Message)
			}
		}
		return r.setPhase(ctx, vm, voxv1alpha1.PhaseDeploying, "waiting for pods to be ready")
	}
}

func (r *VoiceModelReconciler) setPhase(
	ctx context.Context,
	vm *voxv1alpha1.VoiceModel,
	phase voxv1alpha1.VoiceModelPhase,
	message string,
) (ctrl.Result, error) {
	now := metav1.Now()

	if vm.Status.Phase != phase {
		vm.Status.LastTransitionTime = &now
	}
	vm.Status.Phase = phase
	vm.Status.Message = message

	if err := r.Status().Update(ctx, vm); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	if phase != voxv1alpha1.PhaseReady && phase != voxv1alpha1.PhaseFailed {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *VoiceModelReconciler) buildDeployment(vm *voxv1alpha1.VoiceModel) *appsv1.Deployment {
	labels := labelsForVoiceModel(vm)
	replicas := int32(1)
	if vm.Spec.Replicas != nil {
		replicas = *vm.Spec.Replicas
	}

	image := vm.Spec.Image
	if image == "" {
		if vm.Spec.Device == "gpu" {
			image = defaultGPUImage
		} else {
			image = defaultCPUImage
		}
	}

	port := getPort(vm)
	healthPath := "/health"
	initialDelay := int32(60)
	period := int32(10)

	if vm.Spec.Health != nil {
		if vm.Spec.Health.Path != "" {
			healthPath = vm.Spec.Health.Path
		}
		if vm.Spec.Health.InitialDelaySeconds > 0 {
			initialDelay = vm.Spec.Health.InitialDelaySeconds
		}
		if vm.Spec.Health.PeriodSeconds > 0 {
			period = vm.Spec.Health.PeriodSeconds
		}
	}

	resources := vm.Spec.Resources
	if resources == nil {
		resources = defaultResources(vm.Spec.Device)
	}

	env := []corev1.EnvVar{
		{Name: "WHISPER__MODEL", Value: vm.Spec.Model},
		{Name: "WHISPER__INFERENCE_DEVICE", Value: vm.Spec.Device},
		{Name: "WHISPER__COMPUTE_TYPE", Value: vm.Spec.Quantization},
	}

	metricsEnabled := vm.Spec.Metrics == nil || *vm.Spec.Metrics
	annotations := map[string]string{}
	if metricsEnabled {
		annotations["prometheus.io/scrape"] = "true"
		annotations["prometheus.io/port"] = fmt.Sprintf("%d", port)
		annotations["prometheus.io/path"] = "/metrics"
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(vm),
			Namespace: vm.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "inference",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env:       env,
							Resources: *resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: initialDelay,
								PeriodSeconds:       period,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: initialDelay,
								PeriodSeconds:       period,
							},
						},
					},
				},
			},
		},
	}
}

func (r *VoiceModelReconciler) buildService(vm *voxv1alpha1.VoiceModel) *corev1.Service {
	labels := labelsForVoiceModel(vm)
	port := getPort(vm)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName(vm),
			Namespace: vm.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *VoiceModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&voxv1alpha1.VoiceModel{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// --- Helpers ---

func deploymentName(vm *voxv1alpha1.VoiceModel) string {
	return fmt.Sprintf("vox-%s", vm.Name)
}

func serviceName(vm *voxv1alpha1.VoiceModel) string {
	return fmt.Sprintf("vox-%s", vm.Name)
}

func getPort(vm *voxv1alpha1.VoiceModel) int32 {
	if vm.Spec.Port > 0 {
		return vm.Spec.Port
	}
	return 8000
}

func labelsForVoiceModel(vm *voxv1alpha1.VoiceModel) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "voicemodel",
		"app.kubernetes.io/instance":   vm.Name,
		"app.kubernetes.io/managed-by": "voxplatform-operator",
		"vox.io/model":                 vm.Name,
	}
}

func defaultResources(device string) *corev1.ResourceRequirements {
	if device == "gpu" {
		return &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				"nvidia.com/gpu":      resource.MustParse("1"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				"nvidia.com/gpu":      resource.MustParse("1"),
			},
		}
	}

	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1500m"),
			corev1.ResourceMemory: resource.MustParse("3Gi"),
		},
	}
}
