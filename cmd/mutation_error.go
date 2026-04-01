package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var dottedFieldPattern = regexp.MustCompile(`([a-z_]+\.[a-z_]+)`)

type mutationErrorEnvelope struct {
	ErrorCode    string `json:"error_code"`
	Field        string `json:"field,omitempty"`
	Retryable    bool   `json:"retryable"`
	SuggestedFix string `json:"suggested_fix"`
	Message      string `json:"message"`
}

type renderedMutationError struct {
	cause error
}

func (e renderedMutationError) Error() string {
	if e.cause == nil {
		return "mutation error"
	}
	return e.cause.Error()
}

func (e renderedMutationError) Unwrap() error {
	return e.cause
}

func renderMutationError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	var already renderedMutationError
	if errors.As(err, &already) {
		return err
	}
	if format == "json" {
		env := buildMutationErrorEnvelope(err)
		printJSON(cmd, map[string]any{
			"error":          "mutation_failed",
			"error_envelope": env,
		})
	}
	return err
}

func renderMutationValidationError(cmd *cobra.Command, err error, field string, report any) error {
	if err == nil {
		return nil
	}
	if format == "json" {
		env := mutationErrorEnvelope{
			ErrorCode:    "validation_error",
			Field:        strings.TrimSpace(field),
			Retryable:    false,
			SuggestedFix: "Fix invalid input fields and retry.",
			Message:      err.Error(),
		}
		payload := map[string]any{
			"error":          "validation_failed",
			"error_envelope": env,
		}
		if report != nil {
			payload["validation"] = report
		}
		printJSON(cmd, payload)
		return renderedMutationError{cause: err}
	}
	return err
}

func buildMutationErrorEnvelope(err error) mutationErrorEnvelope {
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	env := mutationErrorEnvelope{
		ErrorCode:    "mutation_error",
		Retryable:    false,
		SuggestedFix: "Inspect command output, correct inputs, and retry.",
		Message:      msg,
	}

	switch {
	case strings.Contains(lower, "database is locked"), strings.Contains(lower, "sqlite_busy"):
		env.ErrorCode = "storage_error"
		env.Retryable = true
		env.SuggestedFix = "Retry the command. If it persists, rerun after active writes settle."
	case strings.Contains(lower, "human-only") && strings.Contains(lower, "review state"):
		env.ErrorCode = "transition_error"
		env.Field = "state"
		env.SuggestedFix = "If you are an LLM agent, transition the ticket to the configured review state instead of the completed state when the workflow requires human review."
	case strings.Contains(lower, "cannot transition"), strings.Contains(lower, "cannot advance to"):
		env.ErrorCode = "transition_error"
		env.Field = "state"
		if strings.Contains(lower, "completed state") || strings.Contains(lower, "review state") {
			env.SuggestedFix = "If you are an LLM agent, transition the ticket to the configured review state instead of the completed state when the workflow requires human review."
		} else {
			env.SuggestedFix = "Use a valid state transition from `docket config` workflow states."
		}
	case strings.Contains(lower, "validation failed"),
		strings.Contains(lower, "is required"),
		strings.Contains(lower, "cannot be empty"),
		strings.Contains(lower, "not a valid state"),
		strings.Contains(lower, "invalid"):
		env.ErrorCode = "validation_error"
		env.Field = detectMutationField(msg)
		env.SuggestedFix = "Fix invalid input fields and retry."
	case strings.Contains(lower, "not found"):
		env.ErrorCode = "not_found"
		env.SuggestedFix = "Verify resource identifiers and rerun."
	}

	if env.Field == "" && env.ErrorCode == "validation_error" {
		env.Field = detectMutationField(msg)
	}
	return env
}

func detectMutationField(message string) string {
	if match := dottedFieldPattern.FindStringSubmatch(strings.ToLower(message)); len(match) == 2 {
		return match[1]
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "title"):
		return "title"
	case strings.Contains(lower, "description"), strings.Contains(lower, "--desc"):
		return "description"
	case strings.Contains(lower, "priority"):
		return "priority"
	case strings.Contains(lower, "state"):
		return "state"
	case strings.Contains(lower, "spec"):
		return "spec"
	default:
		return ""
	}
}

func wrapMutationError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", stage, err)
}
