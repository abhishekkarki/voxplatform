// Package v1alpha1 contains the VoiceModel API types.
//
// This defines what a VoiceModel custom resource looks like.
// When someone applies a VoiceModel YAML to the cluster,
// Kubernetes validates it against these types.
//
// The naming convention (v1alpha1) signals API maturity:
// - v1alpha1: experimental, may changes without notice
// - v1beta1: mostly stable, breaking changes possible
// - v1: stable, backwards-compatible changes only

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// VoiceModelSpec defines the desired state of a VoiceModel.
// This is what the user writes in the YAML - "I want this model
// running with these resources."
type VoiceModel struct {
	// Model is the model identifier (e.g., "System/faster-whisper-small.en").
	// The operator uses this to configure the inference server.
	Model string `json:"model"`

	// Replicas is the number of inference pods to run.
	// More replicas = more throughput, but more cost.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Device is the inference device: "cpu" or "gpu".
	// GPU requires a GPU node pool to be available.
	// +kubebuilder:default="cpu"
	// +kubebuilder:validation:Enum=cpu;gpu
	Device string `json:"device,omitempty"`

	// Quantization is the model quantization type.
	// int8 is smaller and faster on CPU but slightly less accurate.
	// +kubebuilder:default="int8"
	// +kubebuilder:validation:Enum=int8;float16;float32
	Quantization string `json:"quantization,omitempty"`

	// Image overrides the default inference server image.
	// If not set, the operator uses sensible defaults based on the model size.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Port is the port the inference server listens on.
	// +kubebuilder:default=8000
	Port int32 `json:"port,omitempty"`

	// Health configures the health check probes.
	Health *HealthConfig `json:"health,omitempty"`

	// Metrics enables Prometheus metrics scraping.
	// +kubebuilder:default=true
	Metrics *bool `json:"metrics,omitempty"`
}

// HealthConfig defines health probe configuration.
type HealthConfig struct {
	// Path for readiness and liveness probes.
	// +kubebuilder:default="/health"
	Path string `json:"path,omitempty"`

	// InitialDelaySeconds before the first probe.
	// Model loading can take 30-120 seconds.
	// +kubebuilder:default=60
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds between probes.
	// +kubebuilder:default=10
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// VoiceModelStatus defines the observed state of a VoiceModel.
// The operator writes this - it reflects what's actually running,
// not what the user asked for.
type VoiceModelStatus struct {
	// Phase is the current lifecycle phase of the VoiceModel.
	// +kubebuilder:validation:Enum=Pending;Deploying;Ready;Failed;Terminating
	Phase VoiceModelPhase `json:"phase,omitempty"`

	// ReadyReplicas is the number of pods that passed readiness checks.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Endpoint is the internal service URL for this model.
	// e.g., "whisper-small.vox.svc.cluster.local:8080"
	Endpoint string `json:"endpoint,omitempty"`

	// Message provides human-readable status details.
	// Useful for debugging - "waiting for model download" or "insufficient GPU"
	Message string `json:"message,omitempty"`

	// Conditions follow the standard Kubernetes conditions pattern.
	// Tools like kubectl and dashboards understand this format.
	Conditions []metav1.Conditions `json:"conditions,omitempty"`

	// LastTransactionTime is when the phase last changed.
	LastTransactionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// VoiceModelPhase represents the lifecycle phase.
type VoiceModelPhase string

const (
	// PhasePending means the VoiceModel has been created but
	// the operator hasn't started reconciling yet.
	PhasePending VoiceModelPhase = "Pending"

	// PhaseDeploying means the operator is creating.updating
	// the Deployment and Service
	PhaseDeploying VoiceModelPhase = "Deploying"

	// PhaseReady means all replicas are running and passing
	// health checks. The model is serving traffic.
	PhaseReady VoiceModelPhase = "Ready"

	// PhaseFailed means the deployment failed - insufficient
	// resources, image pull error, crash loop, etc.
	PhaseFailed VoiceModelPhase = "Failed"

	// PhaseTerminating means the VoiceModel is being deleted
	// and the operator is cleaning up resources.
	PhaseTerminating VoiceModelPhase = "Terminating"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Device",type=string,JSONPath=`.spec.device`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VoiceModel is the Schema for the voicemodels API.
// It represents a deployed voice inference model on the platform.
type VoiceModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VoiceModelSpec   `json:"spec,omitempty"`
	Status VoiceModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VoiceModelList contains a list of VoiceModel resources.
type VoiceModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VoiceModel `json:"items"`
}
