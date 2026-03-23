package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var statusParallel bool

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"st"},
	Short:   "Show runtime ticket state and parallel work safety",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			statusParallel = false
			if f := cmd.Flags().Lookup("parallel"); f != nil {
				f.Changed = false
			}
		}()
		securityMode, _ := securityEnforcementSurface(repo)
		if !statusParallel {
			fmt.Fprintln(cmd.OutOrStdout(), "Runtime status: focused on active ticket/workflow state.")
			fmt.Fprintln(cmd.OutOrStdout(), "Use `docket status --parallel` for active-work ticket matrix.")
			fmt.Fprintf(cmd.OutOrStdout(), "Security enforcement: %s\n", securityMode)
			renderHookStatusSurface(cmd.OutOrStdout())
			return nil
		}

		s := local.New(repo)
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		tickets, err := s.ListTickets(context.Background(), store.Filter{IncludeArchived: true})
		if err != nil {
			return err
		}
		activeTickets := tickets[:0]
		for _, t := range tickets {
			if cfg.StateHasRole(string(t.State), "active") {
				activeTickets = append(activeTickets, t)
			}
		}
		tickets = activeTickets
		relations, _ := loadRelations(repo)
		lockState, _ := refreshLockClaims(repo)
		lockByID := map[string]map[string]bool{}
		for _, l := range lockState.Locks {
			files := map[string]bool{}
			for _, f := range l.Files {
				files[f] = true
			}
			lockByID[l.TicketID] = files
		}

		ids := make([]string, 0, len(tickets))
		for _, t := range tickets {
			ids = append(ids, t.ID)
		}
		sort.Strings(ids)
		fmt.Fprintln(cmd.OutOrStdout(), "Runtime status: parallel matrix (safe/risky):")
		fmt.Fprintf(cmd.OutOrStdout(), "Security enforcement: %s\n", securityMode)
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				a, b := ids[i], ids[j]
				reason := parallelReason(a, b, relations.Relations, lockByID)
				if reason == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  safe:  %s <-> %s\n", a, b)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  risky: %s <-> %s (%s)\n", a, b, reason)
				}
			}
		}
		return nil
	},
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

func parallelReason(a, b string, relations []relationEntry, lockByID map[string]map[string]bool) string {
	for _, r := range relations {
		if r.Relation == "parallel-safe" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return ""
		}
		if r.Relation == "blocks" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return "relation blocks"
		}
		if r.Relation == "depends-on" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return "relation depends-on"
		}
	}
	for f := range lockByID[a] {
		if lockByID[b][f] {
			return "file overlap"
		}
	}
	return ""
}

func init() {
	statusCmd.Flags().BoolVar(&statusParallel, "parallel", false, "show parallel safety matrix")
	rootCmd.AddCommand(statusCmd)
}
