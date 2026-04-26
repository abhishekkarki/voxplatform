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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VoiceModelSpec defines the desired state of a VoiceModel.
type VoiceModelSpec struct {
	// Model is the model identifier (e.g., "Systran/faster-whisper-small.en").
	Model string `json:"model"`

	// Replicas is the number of inference pods to run.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Device is the inference device: "cpu" or "gpu".
	// +kubebuilder:default="cpu"
	// +kubebuilder:validation:Enum=cpu;gpu
	// +optional
	Device string `json:"device,omitempty"`

	// Quantization is the model quantization type.
	// +kubebuilder:default="int8"
	// +kubebuilder:validation:Enum=int8;float16;float32
	// +optional
	Quantization string `json:"quantization,omitempty"`

	// Image overrides the default inference server image.
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines CPU/memory requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Port is the port the inference server listens on.
	// +kubebuilder:default=8000
	// +optional
	Port int32 `json:"port,omitempty"`

	// Health configures the health check probes.
	// +optional
	Health *HealthConfig `json:"health,omitempty"`

	// Metrics enables Prometheus metrics scraping.
	// +kubebuilder:default=true
	// +optional
	Metrics *bool `json:"metrics,omitempty"`
}

// HealthConfig defines health probe configuration.
type HealthConfig struct {
	// Path for readiness and liveness probes.
	// +kubebuilder:default="/health"
	// +optional
	Path string `json:"path,omitempty"`

	// InitialDelaySeconds before the first probe.
	// +kubebuilder:default=60
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds between probes.
	// +kubebuilder:default=10
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// VoiceModelStatus defines the observed state of a VoiceModel.
type VoiceModelStatus struct {
	// Phase is the current lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Deploying;Ready;Failed;Terminating
	// +optional
	Phase VoiceModelPhase `json:"phase,omitempty"`

	// ReadyReplicas is the number of pods that passed readiness checks.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Endpoint is the internal service URL for this model.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Message provides human-readable status details.
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions follow the standard Kubernetes conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastTransitionTime is when the phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// VoiceModelPhase represents the lifecycle phase.
type VoiceModelPhase string

const (
	PhasePending     VoiceModelPhase = "Pending"
	PhaseDeploying   VoiceModelPhase = "Deploying"
	PhaseReady       VoiceModelPhase = "Ready"
	PhaseFailed      VoiceModelPhase = "Failed"
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

func init() {
	SchemeBuilder.Register(&VoiceModel{}, &VoiceModelList{})
}
