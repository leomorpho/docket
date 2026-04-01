package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type smartCommitReport struct {
	TicketID      string        `json:"ticket_id"`
	Ready         bool          `json:"ready"`
	Validated     bool          `json:"validated"`
	CommitMessage string        `json:"commit_message,omitempty"`
	GitCommand    string        `json:"git_command,omitempty"`
	Checks        []wrapUpCheck `json:"checks"`
	NextSteps     []string      `json:"next_steps,omitempty"`
}

var (
	smartCommitMessage  string
	smartCommitValidate string
)

var smartCommitCmd = &cobra.Command{
	Use:   "smart-commit <TKT-NNN>",
	Short: "Generate or validate a closeout-ready commit message with ticket trailer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if normalized, ok := ticket.NormalizeID(id); ok {
			id = normalized
		}

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			cfg = ticket.DefaultConfig()
		}
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		wrapReport, err := buildWrapUpReport(context.Background(), s, cfg, t)
		if err != nil {
			return err
		}
		report := smartCommitReport{
			TicketID:  id,
			Ready:     wrapReport.Ready,
			Checks:    wrapReport.Checks,
			NextSteps: wrapReport.NextSteps,
		}
		if !wrapReport.Ready {
			if format == "json" {
				printJSON(cmd, report)
			}
			return fmt.Errorf("ticket %s is not ready for closeout commit: %s", id, strings.Join(failedWrapChecks(wrapReport.Checks), "; "))
		}

		validateMode := strings.TrimSpace(smartCommitValidate) != ""
		if validateMode {
			msg := strings.TrimSpace(smartCommitValidate)
			if err := validateCommitMessageTrailer(msg, id); err != nil {
				return err
			}
			report.Validated = true
			report.CommitMessage = msg
			report.GitCommand = buildGitCommitCommand(msg, id)
			if format == "json" {
				printJSON(cmd, report)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Smart commit validation: READY for %s\n", id)
			fmt.Fprintln(cmd.OutOrStdout(), "Commit message is valid and includes the required trailer.")
			return nil
		}

		msg := strings.TrimSpace(smartCommitMessage)
		if msg == "" {
			msg = defaultSmartCommitMessage(t)
		}
		msg = withTicketTrailer(msg, id)
		report.CommitMessage = msg
		report.GitCommand = buildGitCommitCommand(msg, id)
		if format == "json" {
			printJSON(cmd, report)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Smart commit for %s: READY\n", id)
		fmt.Fprintln(cmd.OutOrStdout(), "Suggested commit message:")
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		fmt.Fprintln(cmd.OutOrStdout(), "Suggested command:")
		fmt.Fprintln(cmd.OutOrStdout(), report.GitCommand)
		return nil
	},
}

func failedWrapChecks(checks []wrapUpCheck) []string {
	out := []string{}
	for _, check := range checks {
		if check.OK {
			continue
		}
		out = append(out, check.ID)
	}
	return out
}

func validateCommitMessageTrailer(message, ticketID string) error {
	found, ok := extractTicketTrailerID(message)
	if !ok {
		return fmt.Errorf("commit message must include trailer `Ticket: %s`", ticketID)
	}
	if found != ticketID {
		return fmt.Errorf("commit trailer ticket mismatch: got %s want %s", found, ticketID)
	}
	return nil
}

func extractTicketTrailerID(message string) (string, bool) {
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Ticket:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "Ticket:"))
		if normalized, ok := ticket.NormalizeID(value); ok {
			return normalized, true
		}
		return "", false
	}
	return "", false
}

func withTicketTrailer(message, ticketID string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "chore: ticket update"
	}
	found, ok := extractTicketTrailerID(msg)
	if ok && found == ticketID {
		return msg
	}
	if ok && found != ticketID {
		lines := strings.Split(msg, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "Ticket:") {
				continue
			}
			out = append(out, line)
		}
		msg = strings.TrimSpace(strings.Join(out, "\n"))
	}
	if msg == "" {
		return fmt.Sprintf("Ticket: %s", ticketID)
	}
	return msg + "\n\nTicket: " + ticketID
}

func defaultSmartCommitMessage(t *ticket.Ticket) string {
	title := strings.TrimSpace(t.Title)
	if title == "" {
		title = "ticket update"
	}
	return "chore: " + title
}

func buildGitCommitCommand(message, ticketID string) string {
	subject := "chore: ticket update"
	for _, line := range strings.Split(strings.TrimSpace(message), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Ticket:") {
			continue
		}
		subject = trimmed
		break
	}
	return fmt.Sprintf("git commit -m %q -m %q", subject, "Ticket: "+ticketID)
}

func init() {
	smartCommitCmd.Flags().StringVar(&smartCommitMessage, "message", "", "optional commit message seed (trailer auto-added if missing)")
	smartCommitCmd.Flags().StringVar(&smartCommitValidate, "validate", "", "validate an existing commit message for this ticket")
	rootCmd.AddCommand(smartCommitCmd)
}
