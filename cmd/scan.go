package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	g "github.com/leoaudibert/docket/internal/git"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var scanPath string

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan source files for [TKT-NNN] annotations",
	RunE: func(cmd *cobra.Command, args []string) error {
		root := repo
		if scanPath != "" {
			root = filepath.Join(repo, scanPath)
		}

		annotations, err := g.ScanAnnotations(root)
		if err != nil {
			return err
		}

		converted := make([]local.Annotation, 0, len(annotations))
		for _, a := range annotations {
			filePath := a.FilePath
			if scanPath != "" {
				filePath = filepath.ToSlash(filepath.Join(scanPath, a.FilePath))
			}
			converted = append(converted, local.Annotation{TicketID: a.TicketID, FilePath: filePath, LineNum: a.LineNum, Context: a.Context})
		}

		s := local.New(repo)
		if err := s.UpsertAnnotations(context.Background(), converted); err != nil {
			return fmt.Errorf("saving annotations: %w", err)
		}

		files, err := countScannedFiles(root)
		if err != nil {
			return err
		}

		ticketsSet := map[string]struct{}{}
		for _, a := range converted {
			ticketsSet[a.TicketID] = struct{}{}
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"files_scanned": files,
				"annotations":   len(converted),
				"tickets":       len(ticketsSet),
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Scanned %d files. Found %d annotations across %d tickets.\n", files, len(converted), len(ticketsSet))
		return nil
	},
}

func countScannedFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".docket" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		return nil
	})
	return count, err
}

var refsCmd = &cobra.Command{
	Use:   "refs <TKT-NNN>",
	Short: "Show all source locations that reference a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := args[0]
		s := local.New(repo)
		refs, err := s.GetAnnotationsByTicket(context.Background(), ticketID)
		if err != nil {
			return err
		}

		if len(refs) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No annotations found for %s.\n", ticketID)
			return nil
		}

		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].FilePath != refs[j].FilePath {
				return refs[i].FilePath < refs[j].FilePath
			}
			return refs[i].LineNum < refs[j].LineNum
		})

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket_id":  ticketID,
				"references": refs,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s referenced in %d locations:\n\n", ticketID, len(refs))
		for _, r := range refs {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s:%d\n", r.FilePath, r.LineNum)
			fmt.Fprintf(cmd.OutOrStdout(), "    %s\n\n", r.Context)
		}
		return nil
	},
}

func init() {
	scanCmd.Flags().StringVar(&scanPath, "path", "", "optional subdirectory to scan")
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(refsCmd)
}
