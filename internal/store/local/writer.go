package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
	"gopkg.in/yaml.v3"
)

func signTicket(t *ticket.Ticket) error {
	t.WriteHash = "" // Clear existing hash for stable calculation
	content, err := render(t)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(content))
	t.WriteHash = hex.EncodeToString(h[:])
	return nil
}

func validateSignature(t *ticket.Ticket) (bool, error) {
	if t.WriteHash == "" {
		return false, nil
	}
	originalHash := t.WriteHash
	t.WriteHash = "" // Clear to re-calculate
	defer func() { t.WriteHash = originalHash }()

	content, err := render(t)
	if err != nil {
		return false, err
	}
	h := sha256.Sum256([]byte(content))
	calculated := hex.EncodeToString(h[:])
	return calculated == originalHash, nil
}

func render(t *ticket.Ticket) (string, error) {
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	fm := struct {
		ID            string       `yaml:"id"`
		Seq           int          `yaml:"seq"`
		State         ticket.State `yaml:"state"`
		Priority      int          `yaml:"priority"`
		Labels        []string     `yaml:"labels,omitempty"`
		Parent        string       `yaml:"parent,omitempty"`
		BlockedBy     []string     `yaml:"blocked_by,omitempty"`
		Blocks        []string     `yaml:"blocks,omitempty"`
		LinkedCommits []string     `yaml:"linked_commits,omitempty"`
		CreatedAt     string       `yaml:"created_at"`
		UpdatedAt     string       `yaml:"updated_at"`
		StartedAt     string       `yaml:"started_at,omitempty"`
		CompletedAt   string       `yaml:"completed_at,omitempty"`
		CreatedBy     string       `yaml:"created_by"`
		WriteHash     string       `yaml:"write_hash,omitempty"`
	}{
		ID:            t.ID,
		Seq:           t.Seq,
		State:         t.State,
		Priority:      t.Priority,
		Labels:        t.Labels,
		Parent:        t.Parent,
		BlockedBy:     t.BlockedBy,
		Blocks:        t.Blocks,
		LinkedCommits: t.LinkedCommits,
		CreatedAt:     t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		CreatedBy:     t.CreatedBy,
		WriteHash:     t.WriteHash,
	}
	if !t.StartedAt.IsZero() {
		fm.StartedAt = t.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !t.CompletedAt.IsZero() {
		fm.CompletedAt = t.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	sb.Write(fmBytes)
	sb.WriteString("---\n\n")

	// Title
	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", t.ID, t.Title))

	// Description
	if t.Description != "" {
		sb.WriteString("## Description\n")
		sb.WriteString(strings.TrimSpace(t.Description))
		sb.WriteString("\n\n")
	}

	// AC
	if len(t.AC) > 0 {
		sb.WriteString("## Acceptance Criteria\n")
		for _, ac := range t.AC {
			box := "[ ]"
			if ac.Done {
				box = "[x]"
			}
			
			descLines := strings.Split(ac.Description, "\n")
			firstLine := descLines[0]
			
			line := fmt.Sprintf("- %s %s", box, firstLine)
			if ac.Run != "" {
				line += " (run: " + ac.Run + ")"
			}
			if ac.Evidence != "" {
				line += " : " + ac.Evidence
			}
			sb.WriteString(line + "\n")
			
			for i := 1; i < len(descLines); i++ {
				sb.WriteString(descLines[i] + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Plan
	if len(t.Plan) > 0 {
		sb.WriteString("## Plan\n")
		for i, p := range t.Plan {
			descLines := strings.Split(p.Description, "\n")
			firstLine := descLines[0]
			
			sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, p.Status, firstLine))
			if p.Notes != "" {
				sb.WriteString(" : " + p.Notes)
			}
			sb.WriteString("\n")
			
			for j := 1; j < len(descLines); j++ {
				sb.WriteString(descLines[j] + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Comments
	if len(t.Comments) > 0 {
		sb.WriteString("## Comments\n\n")
		for _, c := range t.Comments {
			ts := c.At.UTC().Format("2006-01-02T15:04:05Z")
			sb.WriteString(fmt.Sprintf("### %s — %s\n", ts, c.Author))
			sb.WriteString(strings.TrimSpace(c.Body))
			sb.WriteString("\n\n")
		}
	}

	// Handoff
	if t.Handoff != "" {
		sb.WriteString("## Handoff\n")
		sb.WriteString(strings.TrimSpace(t.Handoff))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
