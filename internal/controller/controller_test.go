package controller

import (
	"os"
	"testing"
)

// TestDecodeRenderedNodeJob ensures the Job manifest produced by the Helm
// chart (captured in testdata) parses into a typed Job and that the GPU
// scheduling knobs survive the round-trip. The fixture is regenerated from
// `helm template` output.
func TestDecodeRenderedNodeJob(t *testing.T) {
	raw, err := os.ReadFile("testdata/node-job.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	job, err := decodeJob(raw, "tdarr-node")
	if err != nil {
		t.Fatalf("decodeJob: %v", err)
	}

	if job.Name != "tdarr-node" {
		t.Errorf("name = %q, want forced name tdarr-node", job.Name)
	}

	spec := job.Spec.Template.Spec
	if spec.RuntimeClassName == nil || *spec.RuntimeClassName != "nvidia" {
		t.Errorf("runtimeClassName not preserved: %v", spec.RuntimeClassName)
	}
	if len(spec.Tolerations) != 1 || spec.Tolerations[0].Key != "nvidia.com/gpu" {
		t.Errorf("gpu toleration not preserved: %+v", spec.Tolerations)
	}
	if len(spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(spec.Containers))
	}
	gpu := spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
	if gpu.Value() != 1 {
		t.Errorf("nvidia.com/gpu limit = %v, want 1", gpu.Value())
	}
}

func TestDecodeRejectsNonJob(t *testing.T) {
	_, err := decodeJob([]byte("apiVersion: v1\nkind: Pod\n"), "x")
	if err == nil {
		t.Fatal("expected error for non-Job manifest")
	}
}
