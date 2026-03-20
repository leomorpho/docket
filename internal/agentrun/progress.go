package agentrun

import (
	"fmt"
	"strconv"
	"strings"
)

type StepStatus string

const (
	StepInProgress StepStatus = "in_progress"
	StepDone       StepStatus = "done"
)

type PlanMarker struct {
	TicketID string `json:"ticket_id"`
	Steps    int    `json:"steps"`
}

type StepMarker struct {
	TicketID string     `json:"ticket_id"`
	Index    int        `json:"index"`
	Status   StepStatus `json:"status"`
	Title    string     `json:"title"`
}

type StatusMarker struct {
	TicketID string `json:"ticket_id"`
	Phase    string `json:"phase"`
}

func ParsePlanLine(line string) (PlanMarker, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "PLAN ") {
		return PlanMarker{}, fmt.Errorf("%w: PLAN prefix is required", ErrInvalidResultLine)
	}
	fields, err := parseKeyValueFields(strings.TrimPrefix(line, "PLAN "))
	if err != nil {
		return PlanMarker{}, err
	}
	steps, err := strconv.Atoi(strings.TrimSpace(fields["steps"]))
	if err != nil || steps <= 0 {
		return PlanMarker{}, fmt.Errorf("%w: steps must be > 0", ErrInvalidResultLine)
	}
	ticketID := strings.TrimSpace(fields["ticket"])
	if ticketID == "" {
		return PlanMarker{}, fmt.Errorf("%w: ticket is required", ErrInvalidResultLine)
	}
	return PlanMarker{TicketID: ticketID, Steps: steps}, nil
}

func ParseStepLine(line string) (StepMarker, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "STEP ") {
		return StepMarker{}, fmt.Errorf("%w: STEP prefix is required", ErrInvalidResultLine)
	}
	fields, err := parseKeyValueFields(strings.TrimPrefix(line, "STEP "))
	if err != nil {
		return StepMarker{}, err
	}
	index, err := strconv.Atoi(strings.TrimSpace(fields["index"]))
	if err != nil || index <= 0 {
		return StepMarker{}, fmt.Errorf("%w: index must be > 0", ErrInvalidResultLine)
	}
	status := StepStatus(strings.TrimSpace(fields["status"]))
	if status != StepInProgress && status != StepDone {
		return StepMarker{}, fmt.Errorf("%w: unsupported step status %q", ErrInvalidResultLine, status)
	}
	ticketID := strings.TrimSpace(fields["ticket"])
	title := strings.TrimSpace(fields["title"])
	if ticketID == "" || title == "" {
		return StepMarker{}, fmt.Errorf("%w: ticket and title are required", ErrInvalidResultLine)
	}
	return StepMarker{TicketID: ticketID, Index: index, Status: status, Title: title}, nil
}

func ParseStatusLine(line string) (StatusMarker, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "STATUS ") {
		return StatusMarker{}, fmt.Errorf("%w: STATUS prefix is required", ErrInvalidResultLine)
	}
	fields, err := parseKeyValueFields(strings.TrimPrefix(line, "STATUS "))
	if err != nil {
		return StatusMarker{}, err
	}
	ticketID := strings.TrimSpace(fields["ticket"])
	phase := strings.TrimSpace(fields["phase"])
	if ticketID == "" || phase == "" {
		return StatusMarker{}, fmt.Errorf("%w: ticket and phase are required", ErrInvalidResultLine)
	}
	return StatusMarker{TicketID: ticketID, Phase: phase}, nil
}

