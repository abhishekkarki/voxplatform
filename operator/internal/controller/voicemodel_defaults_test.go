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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	voxv1alpha1 "github.com/abhishekkarki/voxplatform/operator/api/v1alpha1"
)

// TestDefaultResourcesGPU covers the GPU branch of defaultResources — the
// code path iteration 5 (GPU node pool) depends on. A VoiceModel with
// device: gpu and no explicit spec.resources gets these defaults, and it's
// the nvidia.com/gpu request/limit here (not a nodeSelector or toleration)
// that GKE's ExtendedResourceToleration admission controller and the
// scheduler use to place the pod on the tainted GPU pool — see ADR-010.
func TestDefaultResourcesGPU(t *testing.T) {
	got := defaultResources("gpu")

	const gpuResourceName = corev1.ResourceName("nvidia.com/gpu")

	gpuRequest, ok := got.Requests[gpuResourceName]
	if !ok {
		t.Fatalf("gpu resources: expected a nvidia.com/gpu request, got none in %+v", got.Requests)
	}
	if gpuRequest.Value() != 1 {
		t.Errorf("gpu resources: nvidia.com/gpu request = %s, want 1", gpuRequest.String())
	}

	gpuLimit, ok := got.Limits[gpuResourceName]
	if !ok {
		t.Fatalf("gpu resources: expected a nvidia.com/gpu limit, got none in %+v", got.Limits)
	}
	if gpuLimit.Value() != 1 {
		t.Errorf("gpu resources: nvidia.com/gpu limit = %s, want 1", gpuLimit.String())
	}
}

// TestDefaultResourcesCPU covers the CPU branch — mainly to lock in that it
// does NOT request a GPU (a regression here would make CPU-only VoiceModels
// unschedulable once a GPU pool exists, since the scheduler would then be
// free to consider — and the pod would never request — nvidia.com/gpu).
func TestDefaultResourcesCPU(t *testing.T) {
	got := defaultResources("cpu")

	const gpuResourceName = corev1.ResourceName("nvidia.com/gpu")

	if _, ok := got.Requests[gpuResourceName]; ok {
		t.Errorf("cpu resources: unexpectedly requests nvidia.com/gpu: %+v", got.Requests)
	}
	if _, ok := got.Limits[gpuResourceName]; ok {
		t.Errorf("cpu resources: unexpectedly limits nvidia.com/gpu: %+v", got.Limits)
	}
}

// TestBuildDeploymentGPUImage covers the image-selection branch: a GPU
// VoiceModel with no explicit spec.image should get the CUDA image, not the
// CPU one, so it actually runs on the GPU it requested resources for.
func TestBuildDeploymentGPUImage(t *testing.T) {
	r := &VoiceModelReconciler{}

	vm := &voxv1alpha1.VoiceModel{
		ObjectMeta: metav1.ObjectMeta{Name: "whisper-large-v3-gpu", Namespace: "vox"},
		Spec: voxv1alpha1.VoiceModelSpec{
			Model:        "Systran/faster-whisper-large-v3",
			Device:       "gpu",
			Quantization: "float16",
		},
	}
	deploy := r.buildDeployment(vm)

	containers := deploy.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected exactly one container, got %d", len(containers))
	}
	if got := containers[0].Image; got != defaultGPUImage {
		t.Errorf("gpu VoiceModel image = %q, want %q (defaultGPUImage)", got, defaultGPUImage)
	}
}
