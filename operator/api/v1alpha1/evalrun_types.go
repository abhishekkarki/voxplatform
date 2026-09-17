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

// EvalRunSpec defines the desired state of an EvalRun.
// An EvalRun is a one-shot WER regression check: the operator submits a single
// Argo Workflow that runs the vox-eval harness against a gateway, then reflects
// the resulting pass/fail and metrics back into status. It does not create any
// long-lived resources — deleting the EvalRun garbage-collects its Workflow.
type EvalRunSpec struct {
	// Dataset is the path to the dataset directory (manifest.csv + audio),
	// resolved inside the eval Image — e.g. "datasets/sample".
	Dataset string `json:"dataset"`

	// GatewayURL is the VoxPlatform gateway endpoint to evaluate against,
	// e.g. "http://gateway.vox.svc.cluster.local:8080".
	GatewayURL string `json:"gatewayURL"`

	// Model optionally pins which model the gateway should use for
	// transcription. Empty means the gateway's default model.
	// +optional
	Model string `json:"model,omitempty"`

	// Image is the eval harness container image (bundles vox-eval, the SDK,
	// and the dataset named above).
	Image string `json:"image"`

	// WERThreshold is the maximum acceptable corpus WER; the run is marked
	// Failed if the eval harness reports a higher WER than this.
	// +kubebuilder:default="0.3"
	// +optional
	WERThreshold string `json:"werThreshold,omitempty"`

	// TimeoutSeconds bounds how long the underlying Workflow may run before
	// it is forcibly terminated.
	// +kubebuilder:default=600
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Suspend prevents the controller from submitting a Workflow for this
	// EvalRun. Existing Workflows are left untouched.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// EvalRunStatus defines the observed state of an EvalRun.
type EvalRunStatus struct {
	// Phase is the current lifecycle phase.
	// Succeeded means the eval ran AND its corpus WER was within threshold.
	// Failed covers both a WER regression and an infrastructure failure
	// (e.g. the Workflow errored before producing a report).
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
	// +optional
	Phase EvalRunPhase `json:"phase,omitempty"`

	// WorkflowRef is the name of the Argo Workflow object submitted for this run.
	// +optional
	WorkflowRef string `json:"workflowRef,omitempty"`

	// CorpusWER is the corpus-level Word Error Rate reported by the eval harness.
	// +optional
	CorpusWER string `json:"corpusWER,omitempty"`

	// MeanWER is the mean per-sample Word Error Rate reported by the eval harness.
	// +optional
	MeanWER string `json:"meanWER,omitempty"`

	// Passed is true when CorpusWER was within spec.WERThreshold.
	// +optional
	Passed *bool `json:"passed,omitempty"`

	// TotalSamples is the number of dataset samples evaluated.
	// +optional
	TotalSamples int32 `json:"totalSamples,omitempty"`

	// Message provides human-readable status details.
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime records when the Workflow was submitted.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime records when the Workflow reached a terminal phase.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Conditions follow the standard Kubernetes conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EvalRunPhase represents the lifecycle phase of an EvalRun.
type EvalRunPhase string

const (
	// EvalRunPhasePending means the EvalRun has not yet submitted a Workflow.
	EvalRunPhasePending EvalRunPhase = "Pending"

	// EvalRunPhaseRunning means the Workflow has been submitted and has not
	// yet reached a terminal phase.
	EvalRunPhaseRunning EvalRunPhase = "Running"

	// EvalRunPhaseSucceeded means the eval ran and its corpus WER was
	// within spec.WERThreshold.
	EvalRunPhaseSucceeded EvalRunPhase = "Succeeded"

	// EvalRunPhaseFailed means either the corpus WER exceeded
	// spec.WERThreshold, or the Workflow failed before producing a report.
	EvalRunPhaseFailed EvalRunPhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Passed",type=boolean,JSONPath=`.status.passed`
// +kubebuilder:printcolumn:name="WER",type=string,JSONPath=`.status.corpusWER`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EvalRun is the Schema for the evalruns API.
// It declares a one-shot WER regression check against a gateway, delegates
// execution to an Argo Workflow running the vox-eval harness, and reports the
// resulting pass/fail and metrics.
type EvalRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EvalRunSpec   `json:"spec,omitempty"`
	Status EvalRunStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EvalRunList contains a list of EvalRun resources.
type EvalRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EvalRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EvalRun{}, &EvalRunList{})
}
