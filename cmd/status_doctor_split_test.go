package cmd

import (
	"strings"
	"testing"
)

func TestStatusAndDoctorDescriptionsStayDistinct(t *testing.T) {
	if !strings.Contains(strings.ToLower(statusCmd.Short), "runtime") {
		t.Fatalf("status short description must focus runtime state, got: %q", statusCmd.Short)
	}
	if strings.Contains(strings.ToLower(statusCmd.Short), "integration health") {
		t.Fatalf("status short should not overlap doctor framing, got: %q", statusCmd.Short)
	}

	if !strings.Contains(strings.ToLower(doctorCmd.Short), "integration health") {
		t.Fatalf("doctor short description must focus integration health, got: %q", doctorCmd.Short)
	}
	if strings.Contains(strings.ToLower(doctorCmd.Short), "parallel safety") {
		t.Fatalf("doctor short should not overlap status framing, got: %q", doctorCmd.Short)
	}
}

func TestStatusAndDoctorOutputScopesStayDistinct(t *testing.T) {
	h := newFakeRepoHarness(t)

	statusOut, err := h.run("status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "Runtime status:") {
		t.Fatalf("status output should announce runtime-state scope, got:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "--parallel") {
		t.Fatalf("status output should point to runtime parallel matrix follow-up, got:\n%s", statusOut)
	}

	doctorOut, err := h.run("doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, doctorOut)
	}
	if !strings.Contains(doctorOut, "Setup and integration health") {
		t.Fatalf("doctor output should announce setup/integration-health scope, got:\n%s", doctorOut)
	}
	if strings.Contains(strings.ToLower(doctorOut), "parallel matrix") {
		t.Fatalf("doctor output should not overlap runtime parallel matrix wording, got:\n%s", doctorOut)
	}
}
