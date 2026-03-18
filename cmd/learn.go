package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/leomorpho/docket/internal/learning"
	"github.com/spf13/cobra"
)

type learnEntryView struct {
	Index      int    `json:"index"`
	Category   string `json:"category"`
	Rule       string `json:"rule"`
	Source     string `json:"source"`
	CapturedAt string `json:"captured_at"`
}

var learnCmd = &cobra.Command{
	Use:   "learn",
	Short: "Inspect and search stored learning rules",
}

var (
	learnCaptureCategory string
	learnCaptureRule     string
	learnCaptureSource   string
)

var learnListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored learning rules",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := loadLearnEntries()
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, map[string]any{
				"total":   len(entries),
				"entries": entries,
			})
			return nil
		}
		if len(entries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No stored learn rules.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Stored learn rules (%d):\n", len(entries))
		renderLearnEntriesHuman(cmd, entries)
		return nil
	},
}

var learnSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search stored learning rules",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(args[0])
		entries, err := loadLearnEntries()
		if err != nil {
			return err
		}
		matches := filterLearnEntries(entries, query)
		if format == "json" {
			printJSON(cmd, map[string]any{
				"query":   query,
				"total":   len(matches),
				"entries": matches,
			})
			return nil
		}
		if len(matches) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No learn rules matched %q.\n", query)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Learn search %q (%d matches):\n", query, len(matches))
		renderLearnEntriesHuman(cmd, matches)
		return nil
	},
}

var learnShowCmd = &cobra.Command{
	Use:   "show <index>",
	Short: "Inspect a stored learning rule by list index",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || idx < 1 {
			return fmt.Errorf("index must be a positive integer")
		}
		entries, err := loadLearnEntries()
		if err != nil {
			return err
		}
		if idx > len(entries) {
			return fmt.Errorf("index %d out of range; stored learn rules: %d", idx, len(entries))
		}
		entry := entries[idx-1]
		if format == "json" {
			printJSON(cmd, map[string]any{"entry": entry})
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Learn rule %d:\n", entry.Index)
		fmt.Fprintf(cmd.OutOrStdout(), "  category: %s\n", entry.Category)
		fmt.Fprintf(cmd.OutOrStdout(), "  rule: %s\n", entry.Rule)
		fmt.Fprintf(cmd.OutOrStdout(), "  source: %s\n", displayLearnSource(entry.Source))
		fmt.Fprintf(cmd.OutOrStdout(), "  captured_at: %s\n", displayLearnCapturedAt(entry.CapturedAt))
		return nil
	},
}

var learnCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture a learning rule explicitly",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			learnCaptureCategory = ""
			learnCaptureRule = ""
			learnCaptureSource = ""
		}()
		category := strings.TrimSpace(learnCaptureCategory)
		rule := strings.TrimSpace(learnCaptureRule)
		source := strings.TrimSpace(learnCaptureSource)
		if category == "" {
			return fmt.Errorf("--category is required")
		}
		if rule == "" {
			return fmt.Errorf("--rule is required")
		}
		if source == "" {
			source = "manual:" + detectActor()
		}

		line := fmt.Sprintf("LEARN[%s]: %s", category, rule)
		parsed := learning.Parse(line)
		if len(parsed) == 0 {
			return fmt.Errorf("invalid learning input: expected category and rule text")
		}
		normalizedCategory := parsed[0].Category
		normalizedRule := parsed[0].Rule

		store := learning.NewStore(repo, nil)
		result, err := store.IngestText(source, line)
		if err != nil {
			return err
		}
		entry, ok := findLearnEntry(result.Entries, normalizedCategory, normalizedRule)
		if !ok {
			return fmt.Errorf("captured learn rule missing from store snapshot")
		}
		view := learnEntryView{
			Category:   entry.Category,
			Rule:       entry.Rule,
			Source:     entry.Source,
			CapturedAt: entry.CapturedAt,
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"added": result.Added,
				"total": result.Total,
				"entry": view,
			})
			return nil
		}
		if result.Added == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Learning rule already exists: [%s] %s\n", view.Category, view.Rule)
			fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", displayLearnSource(view.Source))
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Captured learning rule: [%s] %s\n", view.Category, view.Rule)
		fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", displayLearnSource(view.Source))
		return nil
	},
}

func loadLearnEntries() ([]learnEntryView, error) {
	snapshot, err := learning.NewStore(repo, nil).Load()
	if err != nil {
		return nil, err
	}
	entries := make([]learnEntryView, 0, len(snapshot.Entries))
	for i, entry := range snapshot.Entries {
		entries = append(entries, learnEntryView{
			Index:      i + 1,
			Category:   entry.Category,
			Rule:       entry.Rule,
			Source:     entry.Source,
			CapturedAt: entry.CapturedAt,
		})
	}
	return entries, nil
}

func findLearnEntry(entries []learning.Entry, category, rule string) (learning.Entry, bool) {
	targetCategory := strings.ToLower(strings.TrimSpace(category))
	targetRule := strings.ToLower(strings.TrimSpace(rule))
	for _, entry := range entries {
		if strings.ToLower(strings.TrimSpace(entry.Category)) == targetCategory &&
			strings.ToLower(strings.TrimSpace(entry.Rule)) == targetRule {
			return entry, true
		}
	}
	return learning.Entry{}, false
}

func filterLearnEntries(entries []learnEntryView, query string) []learnEntryView {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return entries
	}
	tokens := strings.Fields(strings.ToLower(trimmed))
	if len(tokens) == 0 {
		return entries
	}
	matches := make([]learnEntryView, 0, len(entries))
	for _, entry := range entries {
		haystack := strings.ToLower(entry.Category + " " + entry.Rule + " " + entry.Source)
		ok := true
		for _, token := range tokens {
			if !strings.Contains(haystack, token) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, entry)
		}
	}
	return matches
}

func renderLearnEntriesHuman(cmd *cobra.Command, entries []learnEntryView) {
	for _, entry := range entries {
		fmt.Fprintf(cmd.OutOrStdout(), "%d. [%s] %s\n", entry.Index, entry.Category, entry.Rule)
		fmt.Fprintf(cmd.OutOrStdout(), "   source: %s\n", displayLearnSource(entry.Source))
		fmt.Fprintf(cmd.OutOrStdout(), "   captured_at: %s\n", displayLearnCapturedAt(entry.CapturedAt))
	}
}

func displayLearnSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "(unknown)"
	}
	return source
}

func displayLearnCapturedAt(capturedAt string) string {
	capturedAt = strings.TrimSpace(capturedAt)
	if capturedAt == "" {
		return "(unknown)"
	}
	return capturedAt
}

func init() {
	learnCmd.AddCommand(learnListCmd)
	learnCmd.AddCommand(learnSearchCmd)
	learnCmd.AddCommand(learnShowCmd)
	learnCaptureCmd.Flags().StringVar(&learnCaptureCategory, "category", "", "learning category (for example: reliability, testing)")
	learnCaptureCmd.Flags().StringVar(&learnCaptureRule, "rule", "", "learning rule text")
	learnCaptureCmd.Flags().StringVar(&learnCaptureSource, "source", "", "source metadata (default: manual:<actor>)")
	learnCmd.AddCommand(learnCaptureCmd)
	rootCmd.AddCommand(learnCmd)
}
