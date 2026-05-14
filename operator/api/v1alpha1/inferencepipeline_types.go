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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InferencePipelineSpec defines the desired state of an InferencePipeline.
// A pipeline is an ordered chain of VoiceModel-backed inference steps.
// The operator validates that all referenced VoiceModels exist and are Ready;
// the gateway executes the stages at request time using their service endpoints.
type InferencePipelineSpec struct {
	// Stages is the ordered list of inference steps.
	// Supported names: "stt", "diarize", "summarize".
	// The order determines execution sequence at runtime.
	// +kubebuilder:validation:MinItems=1
	Stages []PipelineStage `json:"stages"`
}

// PipelineStage maps one inference step to a VoiceModel in the same namespace.
type PipelineStage struct {
	// Name identifies this stage type.
	// +kubebuilder:validation:Enum=stt;diarize;summarize
	Name string `json:"name"`

	// Model is the name of a VoiceModel resource in the same namespace.
	// The operator watches this VoiceModel and reflects its readiness here.
	Model string `json:"model"`

	// Enabled controls whether this stage runs at request time.
	// A disabled stage is skipped; its VoiceModel is still validated.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// InferencePipelineStatus defines the observed state of an InferencePipeline.
type InferencePipelineStatus struct {
	// Phase is the current lifecycle phase of the pipeline.
	// Ready = all enabled stages have a Ready VoiceModel.
	// Degraded = some stages are ready, others are not.
	// Failed = a referenced VoiceModel does not exist.
	// +kubebuilder:validation:Enum=Pending;Validating;Ready;Degraded;Failed
	// +optional
	Phase InferencePipelinePhase `json:"phase,omitempty"`

	// Stages shows the readiness of each stage's backing VoiceModel.
	// +optional
	Stages []PipelineStageStatus `json:"stages,omitempty"`

	// Message provides human-readable status details, e.g. "3/3 stages ready".
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions follow the standard Kubernetes conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastTransitionTime records when Phase last changed.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// PipelineStageStatus reflects the readiness of one stage's backing VoiceModel.
type PipelineStageStatus struct {
	// Name of the pipeline stage (stt, diarize, summarize).
	Name string `json:"name"`

	// ModelRef is the VoiceModel name this stage points to.
	ModelRef string `json:"modelRef"`

	// Ready is true when the referenced VoiceModel is in Ready phase.
	Ready bool `json:"ready"`

	// Endpoint is the internal K8s service address resolved from VoiceModel status.
	// Format: <service>.<namespace>.svc.cluster.local:<port>
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Message explains why the stage is or is not ready.
	// +optional
	Message string `json:"message,omitempty"`
}

// InferencePipelinePhase represents the lifecycle phase of a pipeline.
type InferencePipelinePhase string

const (
	// PipelinePhasePending means the pipeline has just been created and
	// the controller has not yet evaluated the referenced VoiceModels.
	PipelinePhasePending InferencePipelinePhase = "Pending"

	// PipelinePhaseValidating means the controller is checking VoiceModel readiness.
	PipelinePhaseValidating InferencePipelinePhase = "Validating"

	// PipelinePhaseReady means all enabled stages have a Ready VoiceModel.
	PipelinePhaseReady InferencePipelinePhase = "Ready"

	// PipelinePhaseDegraded means some stages are Ready but others are not yet.
	// Traffic can still flow through ready stages.
	PipelinePhaseDegraded InferencePipelinePhase = "Degraded"

	// PipelinePhaseFailed means a required VoiceModel does not exist or is in Failed phase.
	PipelinePhaseFailed InferencePipelinePhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InferencePipeline is the Schema for the inferencepipelines API.
// It declares an ordered chain of VoiceModel-backed inference stages
// (STT → diarize → summarize) and tracks their collective readiness.
type InferencePipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferencePipelineSpec   `json:"spec,omitempty"`
	Status InferencePipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InferencePipelineList contains a list of InferencePipeline resources.
type InferencePipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferencePipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferencePipeline{}, &InferencePipelineList{})
}
