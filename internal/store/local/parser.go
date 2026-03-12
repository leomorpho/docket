package local

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
	"gopkg.in/yaml.v3"
)

func parse(content string) (*ticket.Ticket, error) {
	t := &ticket.Ticket{}

	// Split frontmatter
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid ticket format: missing frontmatter")
	}

	fmData := parts[1]
	body := parts[2]

	fm := struct {
		ID            string   `yaml:"id"`
		Seq           int      `yaml:"seq"`
		State         string   `yaml:"state"`
		Priority      int      `yaml:"priority"`
		Labels        []string `yaml:"labels,omitempty"`
		Parent        string   `yaml:"parent,omitempty"`
		BlockedBy     []string `yaml:"blocked_by,omitempty"`
		Blocks        []string `yaml:"blocks,omitempty"`
		LinkedCommits []string `yaml:"linked_commits,omitempty"`
		CreatedAt     string   `yaml:"created_at"`
		UpdatedAt     string   `yaml:"updated_at"`
		CreatedBy     string   `yaml:"created_by"`
		WriteHash     string   `yaml:"write_hash"`
	}{}

	if err := yaml.Unmarshal([]byte(fmData), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	t.ID = fm.ID
	t.Seq = fm.Seq
	t.State = ticket.State(fm.State)
	t.Priority = fm.Priority
	t.Labels = fm.Labels
	t.Parent = fm.Parent
	t.BlockedBy = fm.BlockedBy
	t.Blocks = fm.Blocks
	t.LinkedCommits = fm.LinkedCommits
	t.CreatedBy = fm.CreatedBy
	t.WriteHash = fm.WriteHash

	if fm.CreatedAt != "" {
		t.CreatedAt, _ = time.Parse(time.RFC3339, fm.CreatedAt)
	}
	if fm.UpdatedAt != "" {
		t.UpdatedAt, _ = time.Parse(time.RFC3339, fm.UpdatedAt)
	}

	// Parse body
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentSection string
	var sectionBody []string

	for scanner.Scan() {
		line := scanner.Text()

		// Title (H1)
		if strings.HasPrefix(line, "# ") {
			t.Title = strings.TrimPrefix(line, "# ")
			// Format is usually "# ID: Title"
			if idx := strings.Index(t.Title, ": "); idx != -1 {
				t.Title = t.Title[idx+2:]
			}
			continue
		}

		// Section (H2)
		if strings.HasPrefix(line, "## ") {
			// Save previous section
			processSection(t, currentSection, sectionBody)
			currentSection = strings.TrimPrefix(line, "## ")
			sectionBody = []string{}
			continue
		}

		sectionBody = append(sectionBody, line)
	}
	// Process last section
	processSection(t, currentSection, sectionBody)

	return t, nil
}

func processSection(t *ticket.Ticket, section string, lines []string) {
	content := strings.TrimSpace(strings.Join(lines, "\n"))
	switch strings.ToLower(section) {
	case "description":
		t.Description = content
	case "acceptance criteria":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- [") {
				ac := ticket.AcceptanceCriterion{}
				ac.Done = strings.HasPrefix(line, "- [x]")
				desc := ""
				if len(line) > 6 {
					desc = line[6:]
				}
				if idx := strings.Index(desc, " : "); idx != -1 {
					ac.Evidence = strings.TrimSpace(desc[idx+3:])
					ac.Description = strings.TrimSpace(desc[:idx])
				} else {
					ac.Description = strings.TrimSpace(desc)
				}
				if runIdx := strings.LastIndex(ac.Description, "(run: "); runIdx != -1 && strings.HasSuffix(ac.Description, ")") {
					ac.Run = strings.TrimSpace(strings.TrimSuffix(ac.Description[runIdx+6:], ")"))
					ac.Description = strings.TrimSpace(ac.Description[:runIdx])
				}
				if ac.Description != "" {
					t.AC = append(t.AC, ac)
				}
			}
		}
	case "plan":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Matches "1. [status] description"
			if idx := strings.Index(line, ". ["); idx != -1 && idx < 5 {
				p := ticket.PlanStep{}
				afterNum := line[idx+2:] // "[status] description"
				closeBracket := strings.Index(afterNum, "]")
				if closeBracket != -1 {
					p.Status = afterNum[1:closeBracket]
					desc := strings.TrimSpace(afterNum[closeBracket+1:])
					if nIdx := strings.Index(desc, " : "); nIdx != -1 {
						p.Notes = strings.TrimSpace(desc[nIdx+3:])
						p.Description = strings.TrimSpace(desc[:nIdx])
					} else {
						p.Description = strings.TrimSpace(desc)
					}
					t.Plan = append(t.Plan, p)
				}
			}
		}
	case "comments":
		var currentComment *ticket.Comment
		var commentBody []string

		for _, line := range lines {
			if strings.HasPrefix(line, "### ") {
				// Save previous comment
				if currentComment != nil {
					currentComment.Body = strings.TrimSpace(strings.Join(commentBody, "\n"))
					t.Comments = append(t.Comments, *currentComment)
				}
				header := strings.TrimPrefix(line, "### ")
				parts := strings.SplitN(header, " — ", 2)
				currentComment = &ticket.Comment{}
				if len(parts) == 2 {
					at, _ := time.Parse("2006-01-02T15:04:05Z", parts[0])
					currentComment.At = at
					currentComment.Author = parts[1]
				}
				commentBody = []string{}
			} else if currentComment != nil {
				commentBody = append(commentBody, line)
			}
		}
		if currentComment != nil {
			currentComment.Body = strings.TrimSpace(strings.Join(commentBody, "\n"))
			t.Comments = append(t.Comments, *currentComment)
		}
	case "handoff":
		t.Handoff = content
	}
}
