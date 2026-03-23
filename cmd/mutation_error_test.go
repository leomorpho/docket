package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildMutationErrorEnvelopeValidationTransitionAndStorage(t *testing.T) {
	validation := buildMutationErrorEnvelope(errors.New("ticket.title cannot be empty"))
	if validation.ErrorCode != "validation_error" {
		t.Fatalf("expected validation_error code, got %+v", validation)
	}
	if validation.Field != "ticket.title" && validation.Field != "title" {
		t.Fatalf("expected title field classification, got %+v", validation)
	}
	if validation.Retryable {
		t.Fatalf("validation errors should be non-retryable, got %+v", validation)
	}
	if validation.SuggestedFix == "" {
		t.Fatalf("expected suggested fix for validation errors, got %+v", validation)
	}

	transition := buildMutationErrorEnvelope(errors.New("cannot transition from \"todo\" to \"done\""))
	if transition.ErrorCode != "transition_error" || transition.Field != "state" {
		t.Fatalf("expected transition envelope with state field, got %+v", transition)
	}
	if transition.Retryable {
		t.Fatalf("transition errors should be non-retryable, got %+v", transition)
	}
	if !strings.Contains(transition.SuggestedFix, "`in-review`") {
		t.Fatalf("expected done transition fix to point agents at in-review, got %+v", transition)
	}

	humanOnly := buildMutationErrorEnvelope(errors.New("transition to the configured completed state (default `done`) is human-only. If you are an LLM agent, stop at the configured review state (default `in-review`) instead; that is enough to unblock yourself and hand off for human verification"))
	if humanOnly.ErrorCode != "transition_error" || humanOnly.Field != "state" {
		t.Fatalf("expected human-only transition envelope with state field, got %+v", humanOnly)
	}

	storage := buildMutationErrorEnvelope(errors.New("database is locked (5) (SQLITE_BUSY)"))
	if storage.ErrorCode != "storage_error" {
		t.Fatalf("expected storage_error code, got %+v", storage)
	}
	if !storage.Retryable {
		t.Fatalf("storage errors should be retryable, got %+v", storage)
	}
	if storage.SuggestedFix == "" {
		t.Fatalf("expected suggested fix for storage errors, got %+v", storage)
	}
}
