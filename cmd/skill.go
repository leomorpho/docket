package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
	"github.com/leomorpho/docket/internal/skillusage"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type skillEntry struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Intent   string   `json:"intent"`
	Command  string   `json:"command"`
	Triggers []string `json:"triggers"`
	Optional bool     `json:"optional"`
}

type skillListPayload struct {
	Total            int          `json:"total"`
	MetadataChecksum string       `json:"metadata_checksum"`
	Skills           []skillEntry `json:"skills"`
}

type skillInvokePayload struct {
	SkillID  string `json:"skill_id"`
	TicketID string `json:"ticket_id,omitempty"`
	Command  string `json:"command"`
	Intent   string `json:"intent"`
	Summary  string `json:"summary"`
}

var (
	skillInvokeTicket string
	skillAuditBucket  string
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Discover and invoke canonical Docket skills",
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List canonical Docket skills",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			skillInvokeTicket = ""
			_ = cmd.Flags().Set("ticket", "")
			if f := cmd.Flags().Lookup("ticket"); f != nil {
				f.Changed = false
			}
		}()
		payload, err := loadSkillListPayload(repo)
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, payload)
			return nil
		}
		if len(payload.Skills) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No skills available.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Docket skills (%d):\n", payload.Total)
		for _, entry := range payload.Skills {
			kind := "required"
			if entry.Optional {
				kind = "optional"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", entry.ID, kind)
			fmt.Fprintf(cmd.OutOrStdout(), "  title: %s\n", entry.Title)
			fmt.Fprintf(cmd.OutOrStdout(), "  intent: %s\n", entry.Intent)
			fmt.Fprintf(cmd.OutOrStdout(), "  command: %s\n", entry.Command)
			fmt.Fprintf(cmd.OutOrStdout(), "  triggers: %s\n", strings.Join(entry.Triggers, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "  summary: %s\n", entry.Summary)
		}
		return nil
	},
}

var skillShowCmd = &cobra.Command{
	Use:   "show <skill-id>",
	Short: "Show details for a canonical Docket skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := loadSkillListPayload(repo)
		if err != nil {
			return err
		}
		entry, ok := findSkill(payload.Skills, args[0])
		if !ok {
			return fmt.Errorf("skill %s not found", strings.TrimSpace(args[0]))
		}
		if format == "json" {
			printJSON(cmd, map[string]any{
				"skill":             entry,
				"metadata_checksum": payload.MetadataChecksum,
			})
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Skill: %s\n", entry.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", entry.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Intent: %s\n", entry.Intent)
		fmt.Fprintf(cmd.OutOrStdout(), "Command: %s\n", entry.Command)
		fmt.Fprintf(cmd.OutOrStdout(), "Triggers: %s\n", strings.Join(entry.Triggers, ", "))
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %s\n", entry.Summary)
		fmt.Fprintf(cmd.OutOrStdout(), "Metadata checksum: %s\n", payload.MetadataChecksum)
		return nil
	},
}

var skillInvokeCmd = &cobra.Command{
	Use:   "invoke <skill-id>",
	Short: "Resolve and invoke a canonical Docket skill command",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := loadSkillListPayload(repo)
		if err != nil {
			return err
		}
		entry, ok := findSkill(payload.Skills, args[0])
		if !ok {
			return fmt.Errorf("skill %s not found", strings.TrimSpace(args[0]))
		}
		resolvedTicket := strings.TrimSpace(skillInvokeTicket)
		if normalized, ok := ticket.NormalizeID(resolvedTicket); ok {
			resolvedTicket = normalized
		}
		resolved, err := resolveSkillCommand(entry.Command, resolvedTicket)
		if err != nil {
			return err
		}
		if resolvedTicket != "" {
			// Resolve ticket early so invocation fails fast with actionable feedback.
			t, err := local.New(repo).GetTicket(context.Background(), resolvedTicket)
			if err != nil {
				return err
			}
			if t == nil {
				return fmt.Errorf("ticket %s not found", resolvedTicket)
			}
		}
		if err := skillusage.Append(repo, skillusage.Event{
			SkillID:          entry.ID,
			Source:           skillusage.SourceCLI,
			TicketID:         resolvedTicket,
			Intent:           entry.Intent,
			Command:          resolved,
			MetadataChecksum: payload.MetadataChecksum,
		}); err != nil {
			return err
		}

		out := skillInvokePayload{
			SkillID:  entry.ID,
			TicketID: resolvedTicket,
			Command:  resolved,
			Intent:   entry.Intent,
			Summary:  entry.Summary,
		}
		if format == "json" {
			printJSON(cmd, out)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Skill invocation: %s\n", out.SkillID)
		fmt.Fprintf(cmd.OutOrStdout(), "Command: %s\n", out.Command)
		fmt.Fprintf(cmd.OutOrStdout(), "Intent: %s\n", out.Intent)
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %s\n", out.Summary)
		if out.TicketID != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Ticket: %s\n", out.TicketID)
		}
		return nil
	},
}

var skillAuditCmd = &cobra.Command{
	Use:   "audit [skill-id]",
	Short: "Show skill usage over time",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		events, err := skillusage.Load(repo)
		if err != nil {
			return err
		}
		filter := ""
		if len(args) == 1 {
			filter = args[0]
		}
		audit, err := skillusage.BuildAudit(events, filter, skillusage.BucketSize(strings.ToLower(strings.TrimSpace(skillAuditBucket))))
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, audit)
			return nil
		}
		renderSkillAuditHuman(cmd, audit)
		return nil
	},
}

func loadSkillListPayload(repoRoot string) (skillListPayload, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return skillListPayload{}, err
	}
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		return skillListPayload{}, fmt.Errorf("invalid skill metadata in runtime contract: %#v", report.Errors)
	}
	entries := make([]skillEntry, 0, len(pack.Skills))
	for _, meta := range pack.Skills {
		entries = append(entries, skillEntry{
			ID:       meta.ID,
			Title:    meta.Title,
			Summary:  meta.Summary,
			Intent:   meta.Intent,
			Command:  meta.Command,
			Triggers: append([]string{}, meta.Triggers...),
			Optional: meta.Optional,
		})
	}
	return skillListPayload{
		Total:            len(entries),
		MetadataChecksum: pack.MetadataChecksum,
		Skills:           entries,
	}, nil
}

func findSkill(skills []skillEntry, id string) (skillEntry, bool) {
	target := strings.ToLower(strings.TrimSpace(id))
	for _, entry := range skills {
		if strings.ToLower(entry.ID) == target {
			return entry, true
		}
	}
	return skillEntry{}, false
}

func resolveSkillCommand(template, ticketID string) (string, error) {
	command := strings.TrimSpace(template)
	if command == "" {
		return "", fmt.Errorf("skill command template is empty")
	}
	if strings.Contains(command, "{ticket_id}") {
		if strings.TrimSpace(ticketID) == "" {
			return "", fmt.Errorf("this skill requires --ticket <TKT-NNN>")
		}
		command = strings.ReplaceAll(command, "{ticket_id}", ticketID)
	}
	return command, nil
}

func renderSkillAuditHuman(cmd *cobra.Command, audit skillusage.Audit) {
	out := cmd.OutOrStdout()
	if audit.SkillFilter != "" {
		fmt.Fprintf(out, "Skill usage for %s\n", audit.SkillFilter)
	} else {
		fmt.Fprintln(out, "Skill usage")
	}
	if audit.TotalInvocations == 0 {
		fmt.Fprintln(out, "No skill usage recorded.")
		return
	}
	fmt.Fprintf(out, "Total invocations: %d\n", audit.TotalInvocations)
	fmt.Fprintf(out, "Bucket size: %s\n", audit.BucketSize)
	if audit.From != "" && audit.To != "" {
		fmt.Fprintf(out, "Window: %s to %s\n", audit.From, audit.To)
	}
	if len(audit.Skills) > 0 {
		fmt.Fprintln(out, "By skill:")
		for _, entry := range audit.Skills {
			fmt.Fprintf(out, "- %s: %d\n", entry.ID, entry.Count)
		}
	}
	if len(audit.Timeline) > 0 {
		fmt.Fprintln(out, "Timeline:")
		for _, bucket := range audit.Timeline {
			parts := make([]string, 0, len(bucket.Skills))
			for _, entry := range audit.Skills {
				if count := bucket.Skills[entry.ID]; count > 0 {
					parts = append(parts, fmt.Sprintf("%s=%d", entry.ID, count))
				}
			}
			label := bucket.Start
			if audit.BucketSize == skillusage.BucketWeek && bucket.End != "" && bucket.End != bucket.Start {
				label = bucket.Start + " to " + bucket.End
			}
			if len(parts) == 0 {
				fmt.Fprintf(out, "- %s: total=%d\n", label, bucket.Total)
				continue
			}
			fmt.Fprintf(out, "- %s: total=%d (%s)\n", label, bucket.Total, strings.Join(parts, ", "))
		}
	}
}

func init() {
	skillInvokeCmd.Flags().StringVar(&skillInvokeTicket, "ticket", "", "ticket ID used to resolve {ticket_id} placeholders")
	skillAuditCmd.Flags().StringVar(&skillAuditBucket, "bucket", "auto", "bucket size for audit output (auto, day, week)")
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillShowCmd)
	skillCmd.AddCommand(skillInvokeCmd)
	skillCmd.AddCommand(skillAuditCmd)
	rootCmd.AddCommand(skillCmd)
}
