package local

import (
	"fmt"
	"strings"

	"github.com/leoaudibert/docket/internal/ticket"
	"gopkg.in/yaml.v3"
)

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
		BlockedBy     []string     `yaml:"blocked_by,omitempty"`
		Blocks        []string     `yaml:"blocks,omitempty"`
		LinkedCommits []string     `yaml:"linked_commits,omitempty"`
		CreatedAt     string       `yaml:"created_at"`
		UpdatedAt     string       `yaml:"updated_at"`
		CreatedBy     string       `yaml:"created_by"`
	}{
		ID:            t.ID,
		Seq:           t.Seq,
		State:         t.State,
		Priority:      t.Priority,
		Labels:        t.Labels,
		BlockedBy:     t.BlockedBy,
		Blocks:        t.Blocks,
		LinkedCommits: t.LinkedCommits,
		CreatedAt:     t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		CreatedBy:     t.CreatedBy,
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
			line := fmt.Sprintf("- %s %s", box, ac.Description)
			if ac.Evidence != "" {
				line += " : " + ac.Evidence
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	// Plan
	if len(t.Plan) > 0 {
		sb.WriteString("## Plan\n")
		for i, p := range t.Plan {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, p.Status, p.Description))
			if p.Notes != "" {
				sb.WriteString(" : " + p.Notes)
			}
			sb.WriteString("\n")
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
