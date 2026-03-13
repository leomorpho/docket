package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

// wordCount returns the number of whitespace-separated words in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// Validate checks whether a ticket is schema-valid for this backend.
func (s *Store) Validate(ctx context.Context, id string) ([]store.ValidationError, error) {
	errs, _, err := s.ValidateFile(id)
	return errs, err
}

// ValidateFile validates a single ticket markdown file by ID.
// Returns a list of errors (blocking) and warnings (non-blocking).
func (s *Store) ValidateFile(id string) (errs []store.ValidationError, warns []store.ValidationError, err error) {
	cfg, cfgErr := ticket.LoadConfig(s.RepoRoot)
	if cfgErr != nil {
		cfg = ticket.DefaultConfig()
	}

	path := s.ticketPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ticket file %s: %w", path, err)
	}

	t, parseErr := Parse(string(data))
	if parseErr != nil {
		errs = append(errs, store.ValidationError{Field: "format", Message: parseErr.Error()})
		return errs, warns, nil
	}

	// 1. Required fields
	if t.ID == "" {
		errs = append(errs, store.ValidationError{Field: "id", Message: "required"})
	} else if !strings.HasPrefix(t.ID, "TKT-") {
		errs = append(errs, store.ValidationError{Field: "id", Message: "must match TKT-\\d+"})
	}

	if t.Seq <= 0 {
		errs = append(errs, store.ValidationError{Field: "seq", Message: "must be > 0"})
	}

	if t.State == "" {
		errs = append(errs, store.ValidationError{Field: "state", Message: "required"})
	} else if !cfg.IsValidState(string(t.State)) {
		errs = append(errs, store.ValidationError{Field: "state", Message: fmt.Sprintf("%q is not a valid state", t.State)})
	}

	if t.Priority <= 0 {
		warns = append(warns, store.ValidationError{Field: "priority", Message: "not set or <= 0, defaulting to 10"})
	}

	if t.CreatedAt.IsZero() {
		errs = append(errs, store.ValidationError{Field: "created_at", Message: "required and must be RFC3339"})
	}
	if t.UpdatedAt.IsZero() {
		errs = append(errs, store.ValidationError{Field: "updated_at", Message: "required and must be RFC3339"})
	}
	if t.CreatedBy == "" {
		errs = append(errs, store.ValidationError{Field: "created_by", Message: "required"})
	}

	// 1b. Signature (TKT-147)
	valid, sigErr := validateSignature(t)
	if sigErr != nil {
		errs = append(errs, store.ValidationError{Field: "signature", Message: fmt.Sprintf("validation failed: %v", sigErr)})
	} else if !valid {
		errs = append(errs, store.ValidationError{Field: "signature", Message: "Direct file mutation detected. You must use Docket's MCP tools or CLI to update tickets."})
	}

	// 2. Consistency
	expectedFilename := id + ".md"
	if filepath.Base(path) != expectedFilename {
		errs = append(errs, store.ValidationError{Field: "id", Message: fmt.Sprintf("ID %q does not match filename %q", t.ID, expectedFilename)})
	} else if t.ID != id {
		errs = append(errs, store.ValidationError{Field: "id", Message: fmt.Sprintf("ID in file %q does not match ID in filename %q", t.ID, id)})
	}

	// 3. References
	if t.Parent != "" {
		pPath := s.ticketPath(t.Parent)
		if _, err := os.Stat(pPath); os.IsNotExist(err) {
			errs = append(errs, store.ValidationError{Field: "parent", Message: fmt.Sprintf("referenced parent ticket %q does not exist", t.Parent)})
		}
	}
	for i, bid := range t.BlockedBy {
		bPath := s.ticketPath(bid)
		if _, err := os.Stat(bPath); os.IsNotExist(err) {
			errs = append(errs, store.ValidationError{Field: fmt.Sprintf("blocked_by[%d]", i), Message: fmt.Sprintf("referenced ticket %q does not exist", bid)})
		}
	}

	// 4. Structure
	if t.Description == "" {
		errs = append(errs, store.ValidationError{Field: "body", Message: "## Description section is required"})
	}
	if len(t.AC) == 0 {
		errs = append(errs, store.ValidationError{Field: "body", Message: "## Acceptance Criteria section is required"})
	}

	// 5. Warnings — states that require a handoff are determined by which states
	// are closed (open: false) and meaningful for review. Use "in-review" and "done"
	// as the handoff-required states (these are fixed workflow semantics, not config-driven yet).
	requiresHandoff := t.State == "in-review" || t.State == "done"
	if t.Handoff == "" {
		if requiresHandoff {
			errs = append(errs, store.ValidationError{Field: "handoff", Message: "## Handoff section is required for in-review and done tickets"})
		}
		warns = append(warns, store.ValidationError{Field: "handoff", Message: "## Handoff section missing (recommended)"})
	} else if requiresHandoff {
		for _, missing := range missingHandoffSections(cfg, t.Handoff) {
			errs = append(errs, store.ValidationError{Field: "handoff", Message: fmt.Sprintf("missing required subsection: %s", missing)})
		}
	}

	if len(t.Labels) == 0 {
		warns = append(warns, store.ValidationError{Field: "labels", Message: "labels is empty"})
	}

	// Quality warnings
	if len(t.AC) < 2 {
		warns = append(warns, store.ValidationError{Field: "quality.ac", Message: "fewer than 2 acceptance criteria — add specific testable outcomes with: docket ac add"})
	}

	wc := wordCount(t.Description)
	if wc < 20 {
		warns = append(warns, store.ValidationError{Field: "quality.desc", Message: fmt.Sprintf("description is short (%d words) — consider adding more context so agents can execute without clarification", wc)})
	}

	if t.State == "in-progress" && len(t.Comments) == 0 {
		warns = append(warns, store.ValidationError{Field: "quality.comments", Message: "ticket is in-progress with no comments — add reasoning with: docket comment TKT-NNN --body '...'"})
	}

	return errs, warns, nil
}

// ValidateAll validates all tickets in .docket/tickets/.
func (s *Store) ValidateAll(ctx context.Context) (map[string][]store.ValidationError, map[string][]store.ValidationError, error) {
	ticketsDir := filepath.Join(s.RepoRoot, ".docket", "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	allErrs := make(map[string][]store.ValidationError)
	allWarns := make(map[string][]store.ValidationError)

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			id := strings.TrimSuffix(entry.Name(), ".md")
			errs, warns, err := s.ValidateFile(id)
			if err != nil {
				return nil, nil, err
			}
			if len(errs) > 0 {
				allErrs[id] = errs
			}
			if len(warns) > 0 {
				allWarns[id] = warns
			}
		}
	}

	// 6. Cycle detection
	if cycleErr := s.detectCycles(); cycleErr != nil {
		allErrs["global"] = append(allErrs["global"], store.ValidationError{Field: "dependencies", Message: cycleErr.Error()})
	}

	return allErrs, allWarns, nil
}

func (s *Store) detectCycles() error {
	ticketsDir := filepath.Join(s.RepoRoot, ".docket", "tickets")
	entries, _ := os.ReadDir(ticketsDir)

	adj := make(map[string][]string)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			id := strings.TrimSuffix(entry.Name(), ".md")
			t, err := s.GetTicket(context.Background(), id)
			if err == nil && t != nil {
				adj[id] = t.BlockedBy
			}
		}
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var check func(u string) error
	check = func(u string) error {
		visited[u] = true
		recStack[u] = true

		for _, v := range adj[u] {
			if !visited[v] {
				if err := check(v); err != nil {
					return err
				}
			} else if recStack[v] {
				return fmt.Errorf("cycle detected involving %s and %s", u, v)
			}
		}

		recStack[u] = false
		return nil
	}

	for id := range adj {
		if !visited[id] {
			if err := check(id); err != nil {
				return err
			}
		}
	}

	return nil
}

// missingHandoffSections returns which required sections are absent from the handoff text.
// Required sections come from cfg.HandoffSections.
func missingHandoffSections(cfg *ticket.Config, handoff string) []string {
	lower := strings.ToLower(handoff)
	var missing []string
	for _, section := range cfg.HandoffSections {
		if !strings.Contains(lower, strings.ToLower(section)) {
			missing = append(missing, section)
		}
	}
	return missing
}
