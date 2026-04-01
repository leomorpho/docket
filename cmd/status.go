package cmd

import (
	"context"
	"fmt"
	"strings"

	selectorpkg "github.com/leomorpho/docket/internal/agentrun/selector"
	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show runtime ticket state and runnable queue status",
	RunE: func(cmd *cobra.Command, args []string) error {
		securityMode, _ := securityEnforcementSurface(repo)
		fmt.Fprintln(cmd.OutOrStdout(), "Runtime status: focused on active ticket/workflow state.")
		queueLine, err := buildQueueStatusLine(context.Background(), repo)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), queueLine)
		fmt.Fprintf(cmd.OutOrStdout(), "Security enforcement: %s\n", securityMode)
		renderHookStatusSurface(cmd.OutOrStdout())
		return nil
	},
}

func buildQueueStatusLine(ctx context.Context, repoRoot string) (string, error) {
	s := local.New(repoRoot)
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return "", err
	}
	selection, err := selectorpkg.New(selectorpkg.Dependencies{
		Store:      s,
		LoadConfig: func(string) (*ticket.Config, error) { return cfg, nil },
	}).Next(ctx)
	if err != nil {
		return "", err
	}
	if !selection.Found {
		return fmt.Sprintf("Queue: %s", selection.Reason), nil
	}

	tickets, err := workablepkg.Tickets(ctx, s, cfg, store.Filter{})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Queue: next runnable ticket is %s (%d runnable now).", selection.TicketID, len(tickets)), nil
}

func renderHookStatusSurface(out interface{ Write([]byte) (int, error) }) {
	status, err := buildHookStatusView(repo)
	if err != nil {
		fmt.Fprintf(out, "Hooks: unknown (%v)\n", err)
		return
	}
	fmt.Fprintf(out, "Hooks: %s\n", status.Readiness)

	recent := recentBlockingHookEvents(repo, 3)
	if status.Ready && len(recent) == 0 {
		return
	}

	if len(status.Events) > 0 {
		fmt.Fprintf(out, "Hook policy: %s\n", compactHookModes(status.Events))
	}
	if len(recent) > 0 {
		fmt.Fprintln(out, "Recent blocking hook events:")
		for _, item := range recent {
			fmt.Fprintf(out, "  - %s\n", item)
		}
	}
	if !status.Ready {
		fmt.Fprintln(out, "Hook remediation: run `docket bootstrap` to repair hook wiring.")
	}
}

func compactHookModes(events []hookEventView) string {
	parts := make([]string, 0, len(events))
	for _, event := range events {
		parts = append(parts, fmt.Sprintf("%s (%s)", event.Name, event.Mode))
	}
	return strings.Join(parts, ", ")
}

func recentBlockingHookEvents(repoRoot string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	events, err := lifecycle.Load(repoRoot)
	if err != nil || len(events) == 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for i := len(events) - 1; i >= 0 && len(out) < limit; i-- {
		ev := events[i]
		if ev.Type != lifecycle.EventToolFailure {
			continue
		}
		tool, _ := ev.Payload["tool"].(string)
		if tool == "" || !strings.Contains(strings.ToLower(tool), "hook") {
			continue
		}
		errText, _ := ev.Payload["error"].(string)
		if errText == "" {
			errText = "hook failure"
		}
		out = append(out, fmt.Sprintf("%s: %s", tool, errText))
	}
	return out
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
