package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestMutatingCommandsEmitStructuredJSONErrorEnvelope(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-970", 970, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	badSpec := map[string]any{
		"version": "docket.apply/v1",
		"ticket": map[string]any{
			"title": 7,
		},
	}
	badSpecPath := h.writeJSONSpec("errors/bad-ticket-spec.json", badSpec)
	validationOut, err := h.run("--format", "json", "ticket", "apply", "--spec", badSpecPath)
	if err == nil {
		t.Fatalf("expected validation failure for bad ticket spec, output=%s", validationOut)
	}

	validationJSON, err := extractFirstJSONObject(validationOut)
	if err != nil {
		t.Fatalf("extract validation json failed: %v\n%s", err, validationOut)
	}
	var validationPayload map[string]any
	if err := json.Unmarshal([]byte(validationJSON), &validationPayload); err != nil {
		t.Fatalf("expected json error envelope output, got err=%v output=%s", err, validationOut)
	}
	validationEnvelope, ok := validationPayload["error_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("expected error_envelope for validation failure, got %#v", validationPayload)
	}
	if validationEnvelope["error_code"] != "validation_error" {
		t.Fatalf("expected validation error code, got %#v", validationEnvelope)
	}
	if validationEnvelope["suggested_fix"] == "" {
		t.Fatalf("expected suggested_fix in validation envelope, got %#v", validationEnvelope)
	}

	transitionOut, err := h.run("--format", "json", "update", "TKT-970", "--state", "in-review")
	if err == nil {
		t.Fatalf("expected transition failure, output=%s", transitionOut)
	}
	transitionJSON, err := extractFirstJSONObject(transitionOut)
	if err != nil {
		t.Fatalf("extract transition json failed: %v\n%s", err, transitionOut)
	}
	var transitionPayload map[string]any
	if err := json.Unmarshal([]byte(transitionJSON), &transitionPayload); err != nil {
		t.Fatalf("expected json error envelope output, got err=%v output=%s", err, transitionOut)
	}
	transitionEnvelope, ok := transitionPayload["error_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("expected error_envelope for transition failure, got %#v", transitionPayload)
	}
	if transitionEnvelope["error_code"] != "transition_error" {
		t.Fatalf("expected transition error code, got %#v", transitionEnvelope)
	}
	if transitionEnvelope["field"] != "state" {
		t.Fatalf("expected transition field state, got %#v", transitionEnvelope)
	}
	if transitionEnvelope["suggested_fix"] == "" {
		t.Fatalf("expected suggested_fix in transition envelope, got %#v", transitionEnvelope)
	}

	validationFixture := h.writeFixture("errors/validation-failure.json", []byte(validationOut))
	transitionFixture := h.writeFixture("errors/transition-failure.json", []byte(transitionOut))
	t.Logf("error fixtures: %s | %s", validationFixture, transitionFixture)
}

func extractFirstJSONObject(raw string) (string, error) {
	start := -1
	depth := 0
	for i, ch := range raw {
		if ch == '{' {
			if start == -1 {
				start = i
			}
			depth++
			continue
		}
		if ch == '}' {
			if start == -1 {
				continue
			}
			depth--
			if depth == 0 {
				return raw[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("json object not found")
}
