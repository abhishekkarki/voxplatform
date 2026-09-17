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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	voxv1alpha1 "github.com/abhishekkarki/voxplatform/operator/api/v1alpha1"
)

var _ = Describe("EvalRun Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-evalrun"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var reconciler *EvalRunReconciler

		newEvalRun := func(spec voxv1alpha1.EvalRunSpec) *voxv1alpha1.EvalRun {
			return &voxv1alpha1.EvalRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: spec,
			}
		}

		doReconcile := func() {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			reconciler = &EvalRunReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		})

		AfterEach(func() {
			evalRun := &voxv1alpha1.EvalRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, evalRun); err == nil {
				Expect(k8sClient.Delete(ctx, evalRun)).To(Succeed())
			}

			wf := &unstructured.Unstructured{}
			wf.SetGroupVersionKind(workflowGVK)
			if err := k8sClient.Get(ctx, typeNamespacedName, wf); err == nil {
				Expect(k8sClient.Delete(ctx, wf)).To(Succeed())
			}
		})

		It("submits an owned Workflow and marks the EvalRun Running", func() {
			resource := newEvalRun(voxv1alpha1.EvalRunSpec{
				Dataset:    "datasets/sample",
				GatewayURL: "http://gateway.vox.svc.cluster.local:8080",
				Image:      "vox-eval:latest",
			})
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			doReconcile()

			updated := &voxv1alpha1.EvalRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(voxv1alpha1.EvalRunPhaseRunning))
			Expect(updated.Status.WorkflowRef).To(Equal(resourceName))
			Expect(updated.Status.StartTime).NotTo(BeNil())

			wf := &unstructured.Unstructured{}
			wf.SetGroupVersionKind(workflowGVK)
			Expect(k8sClient.Get(ctx, typeNamespacedName, wf)).To(Succeed())

			owners := wf.GetOwnerReferences()
			Expect(owners).To(HaveLen(1))
			Expect(owners[0].Name).To(Equal(resourceName))
			Expect(owners[0].Kind).To(Equal("EvalRun"))
		})

		It("marks the EvalRun Succeeded when the report shows the corpus WER within threshold", func() {
			resource := newEvalRun(voxv1alpha1.EvalRunSpec{
				Dataset:      "datasets/sample",
				GatewayURL:   "http://gateway.vox.svc.cluster.local:8080",
				Image:        "vox-eval:latest",
				WERThreshold: "0.3",
			})
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			doReconcile()

			report, err := json.Marshal(map[string]interface{}{
				"passed": true, "corpus_wer": 0.12, "mean_wer": 0.15, "total_samples": 3,
			})
			Expect(err).NotTo(HaveOccurred())
			simulateWorkflowCompletion(ctx, typeNamespacedName, "Succeeded", string(report))

			doReconcile()

			updated := &voxv1alpha1.EvalRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(voxv1alpha1.EvalRunPhaseSucceeded))
			Expect(updated.Status.CorpusWER).To(Equal("0.1200"))
			Expect(updated.Status.MeanWER).To(Equal("0.1500"))
			Expect(updated.Status.TotalSamples).To(Equal(int32(3)))
			Expect(updated.Status.Passed).NotTo(BeNil())
			Expect(*updated.Status.Passed).To(BeTrue())
			Expect(updated.Status.CompletionTime).NotTo(BeNil())
		})

		It("marks the EvalRun Failed on a WER regression even when the Workflow itself reports Succeeded", func() {
			// vox-eval exits 1 when the corpus WER exceeds the threshold, which
			// fails the underlying Argo step/Workflow too — but this test
			// simulates the (also valid) case where Argo's own phase is still
			// Succeeded to prove the controller derives Phase from the report's
			// `passed` field, not from the raw Workflow phase.
			resource := newEvalRun(voxv1alpha1.EvalRunSpec{
				Dataset:      "datasets/sample",
				GatewayURL:   "http://gateway.vox.svc.cluster.local:8080",
				Image:        "vox-eval:latest",
				WERThreshold: "0.1",
			})
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			doReconcile()

			report, err := json.Marshal(map[string]interface{}{
				"passed": false, "corpus_wer": 0.42, "mean_wer": 0.5, "total_samples": 3,
			})
			Expect(err).NotTo(HaveOccurred())
			simulateWorkflowCompletion(ctx, typeNamespacedName, "Succeeded", string(report))

			doReconcile()

			updated := &voxv1alpha1.EvalRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(voxv1alpha1.EvalRunPhaseFailed))
			Expect(updated.Status.CorpusWER).To(Equal("0.4200"))
			Expect(updated.Status.Passed).NotTo(BeNil())
			Expect(*updated.Status.Passed).To(BeFalse())
		})

		It("marks the EvalRun Failed when the Workflow errors before producing a report", func() {
			resource := newEvalRun(voxv1alpha1.EvalRunSpec{
				Dataset:    "datasets/sample",
				GatewayURL: "http://gateway.vox.svc.cluster.local:8080",
				Image:      "vox-eval:latest",
			})
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			doReconcile()

			simulateWorkflowCompletion(ctx, typeNamespacedName, "Error", "")

			doReconcile()

			updated := &voxv1alpha1.EvalRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(voxv1alpha1.EvalRunPhaseFailed))
			Expect(updated.Status.Passed).To(BeNil())
		})

		It("never submits a Workflow when suspended", func() {
			resource := newEvalRun(voxv1alpha1.EvalRunSpec{
				Dataset:    "datasets/sample",
				GatewayURL: "http://gateway.vox.svc.cluster.local:8080",
				Image:      "vox-eval:latest",
				Suspend:    true,
			})
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			doReconcile()

			updated := &voxv1alpha1.EvalRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(voxv1alpha1.EvalRunPhasePending))
			Expect(updated.Status.WorkflowRef).To(BeEmpty())

			wf := &unstructured.Unstructured{}
			wf.SetGroupVersionKind(workflowGVK)
			err := k8sClient.Get(ctx, typeNamespacedName, wf)
			Expect(err).To(HaveOccurred())
		})
	})
})

// simulateWorkflowCompletion patches the status subresource of the Workflow
// already submitted for typeNamespacedName, standing in for what the (not
// installed, in these tests) real Argo Workflows controller would do.
// reportJSON, when non-empty, is set as the eval-report output parameter's
// value, matching how Argo's wait sidecar captures declared output
// parameters after the main container exits.
func simulateWorkflowCompletion(ctx context.Context, name types.NamespacedName, phase, reportJSON string) {
	wf := &unstructured.Unstructured{}
	wf.SetGroupVersionKind(workflowGVK)
	Expect(k8sClient.Get(ctx, name, wf)).To(Succeed())

	status := map[string]interface{}{
		"phase": phase,
	}
	if reportJSON != "" {
		status["outputs"] = map[string]interface{}{
			"parameters": []interface{}{
				map[string]interface{}{
					"name":  evalReportOutputParam,
					"value": reportJSON,
				},
			},
		}
	}
	wf.Object["status"] = status

	Expect(k8sClient.Status().Update(ctx, wf)).To(Succeed())
}
