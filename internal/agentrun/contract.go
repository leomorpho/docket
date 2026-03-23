package agentrun

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Role string

const (
	RoleImplementer Role = "implementer"
	RoleReviewer    Role = "reviewer"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusStuck   Status = "stuck"
	StatusFailed  Status = "failed"
)

func (s Status) IsTerminal() bool {
	switch s {
	case StatusDone, StatusStuck, StatusFailed:
		return true
	default:
		return false
	}
}

type ReviewStatus string

const (
	ReviewApproved        ReviewStatus = "approved"
	ReviewChangesRequired ReviewStatus = "changes_required"
)

var (
	ErrInvalidResultLine = errors.New("invalid result line")
	ErrInvalidRunSpec    = errors.New("invalid run spec")
	ErrInvalidRunRecord  = errors.New("invalid run record")
)

type RunSpec struct {
	TicketID     string            `json:"ticket_id"`
	Role         Role              `json:"role"`
	RepoRoot     string            `json:"repo_root"`
	WorktreePath string            `json:"worktree_path"`
	Branch       string            `json:"branch"`
	Prompt       string            `json:"prompt"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (s RunSpec) Validate() error {
	if strings.TrimSpace(s.TicketID) == "" {
		return fmt.Errorf("%w: ticket_id is required", ErrInvalidRunSpec)
	}
	switch s.Role {
	case RoleImplementer, RoleReviewer:
	default:
		return fmt.Errorf("%w: unsupported role %q", ErrInvalidRunSpec, s.Role)
	}
	if strings.TrimSpace(s.RepoRoot) == "" {
		return fmt.Errorf("%w: repo_root is required", ErrInvalidRunSpec)
	}
	if strings.TrimSpace(s.WorktreePath) == "" {
		return fmt.Errorf("%w: worktree_path is required", ErrInvalidRunSpec)
	}
	if strings.TrimSpace(s.Branch) == "" {
		return fmt.Errorf("%w: branch is required", ErrInvalidRunSpec)
	}
	if strings.TrimSpace(s.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidRunSpec)
	}
	return nil
}

type RunRecord struct {
	TicketID     string `json:"ticket_id"`
	Role         Role   `json:"role"`
	Adapter      string `json:"adapter"`
	RepoRoot     string `json:"repo_root"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	StartedAt    string `json:"started_at"`
	SessionID    string `json:"session_id"`
}

func (r RunRecord) Validate() error {
	if strings.TrimSpace(r.TicketID) == "" {
		return fmt.Errorf("%w: ticket_id is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(string(r.Role)) == "" {
		return fmt.Errorf("%w: role is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(r.Adapter) == "" {
		return fmt.Errorf("%w: adapter is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(r.RepoRoot) == "" {
		return fmt.Errorf("%w: repo_root is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(r.WorktreePath) == "" {
		return fmt.Errorf("%w: worktree_path is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(r.Branch) == "" {
		return fmt.Errorf("%w: branch is required", ErrInvalidRunRecord)
	}
	if strings.TrimSpace(r.StartedAt) == "" {
		return fmt.Errorf("%w: started_at is required", ErrInvalidRunRecord)
	}
	if _, err := time.Parse(time.RFC3339Nano, r.StartedAt); err != nil {
		return fmt.Errorf("%w: started_at must be RFC3339Nano: %v", ErrInvalidRunRecord, err)
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return fmt.Errorf("%w: session_id is required", ErrInvalidRunRecord)
	}
	return nil
}

type Result struct {
	Status    Status `json:"status"`
	TicketID  string `json:"ticket_id"`
	Role      Role   `json:"role"`
	CommitSHA string `json:"commit,omitempty"`
	Tests     string `json:"tests,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (r Result) Validate() error {
	if strings.TrimSpace(r.TicketID) == "" {
		return fmt.Errorf("%w: ticket is required", ErrInvalidResultLine)
	}
	switch r.Role {
	case RoleImplementer, RoleReviewer:
	default:
		return fmt.Errorf("%w: unsupported role %q", ErrInvalidResultLine, r.Role)
	}
	switch r.Status {
	case StatusDone:
		if strings.TrimSpace(r.CommitSHA) == "" && r.Role == RoleImplementer {
			return fmt.Errorf("%w: commit is required for done implementer results", ErrInvalidResultLine)
		}
		if strings.TrimSpace(r.Reason) != "" {
			return fmt.Errorf("%w: done results must not include reason", ErrInvalidResultLine)
		}
	case StatusStuck, StatusFailed:
		if strings.TrimSpace(r.Reason) == "" {
			return fmt.Errorf("%w: reason is required for %s results", ErrInvalidResultLine, r.Status)
		}
		if strings.TrimSpace(r.CommitSHA) != "" {
			return fmt.Errorf("%w: %s results must not include commit", ErrInvalidResultLine, r.Status)
		}
	case StatusRunning:
		return fmt.Errorf("%w: running is not a terminal RESULT status", ErrInvalidResultLine)
	default:
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidResultLine, r.Status)
	}
	return nil
}

func (r Result) Line() string {
	fields := map[string]string{
		"status": string(r.Status),
		"ticket": r.TicketID,
		"role":   string(r.Role),
	}
	if strings.TrimSpace(r.CommitSHA) != "" {
		fields["commit"] = r.CommitSHA
	}
	if strings.TrimSpace(r.Tests) != "" {
		fields["tests"] = r.Tests
	}
	if strings.TrimSpace(r.Reason) != "" {
		fields["reason"] = r.Reason
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, "RESULT")
	for _, key := range keys {
		value := quoteResultValue(fields[key])
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, " ")
}

func ParseResultLine(line string) (Result, error) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "RESULT ") {
		return Result{}, fmt.Errorf("%w: RESULT prefix is required", ErrInvalidResultLine)
	}
	fields, err := parseKeyValueFields(strings.TrimPrefix(line, "RESULT "))
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Status:    Status(fields["status"]),
		TicketID:  fields["ticket"],
		Role:      Role(fields["role"]),
		CommitSHA: fields["commit"],
		Tests:     fields["tests"],
		Reason:    fields["reason"],
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func quoteResultValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

type ReviewResult struct {
	Status          ReviewStatus `json:"status"`
	TicketID        string       `json:"ticket_id"`
	Role            Role         `json:"role"`
	RequiredChanges string       `json:"required_changes,omitempty"`
}

func (r ReviewResult) Validate() error {
	if strings.TrimSpace(r.TicketID) == "" {
		return fmt.Errorf("%w: ticket is required", ErrInvalidResultLine)
	}
	if r.Role != RoleReviewer {
		return fmt.Errorf("%w: review role must be reviewer", ErrInvalidResultLine)
	}
	switch r.Status {
	case ReviewApproved:
		if strings.TrimSpace(r.RequiredChanges) != "" {
			return fmt.Errorf("%w: approved review must not include required_changes", ErrInvalidResultLine)
		}
	case ReviewChangesRequired:
		if strings.TrimSpace(r.RequiredChanges) == "" {
			return fmt.Errorf("%w: changes_required review must include required_changes", ErrInvalidResultLine)
		}
	default:
		return fmt.Errorf("%w: unsupported review status %q", ErrInvalidResultLine, r.Status)
	}
	return nil
}

func (r ReviewResult) Line() string {
	fields := map[string]string{
		"status": string(r.Status),
		"ticket": r.TicketID,
		"role":   string(r.Role),
	}
	if strings.TrimSpace(r.RequiredChanges) != "" {
		fields["required_changes"] = r.RequiredChanges
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, "REVIEW")
	for _, key := range keys {
		parts = append(parts, key+"="+quoteResultValue(fields[key]))
	}
	return strings.Join(parts, " ")
}

func ParseReviewLine(line string) (ReviewResult, error) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "REVIEW ") {
		return ReviewResult{}, fmt.Errorf("%w: REVIEW prefix is required", ErrInvalidResultLine)
	}
	fields, err := parseKeyValueFields(strings.TrimPrefix(line, "REVIEW "))
	if err != nil {
		return ReviewResult{}, err
	}
	result := ReviewResult{
		Status:          ReviewStatus(fields["status"]),
		TicketID:        fields["ticket"],
		Role:            Role(fields["role"]),
		RequiredChanges: fields["required_changes"],
	}
	if err := result.Validate(); err != nil {
		return ReviewResult{}, err
	}
	return result, nil
}
