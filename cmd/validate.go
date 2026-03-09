package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var (
	showWarns bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [TKT-NNN]",
	Short: "Validate ticket schema and dependencies",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		s := local.New(repo)
		ctx := context.Background()

		if len(args) == 1 {
			id := args[0]
			errs, warns, err := s.ValidateFile(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(2)
			}

			if format == "json" {
				printJSON(map[string]interface{}{
					"valid":   len(errs) == 0,
					"errors":  errs,
					"warnings": warns,
				})
			} else {
				if len(errs) == 0 {
					fmt.Printf("✓ %s valid\n", id)
					if showWarns && len(warns) > 0 {
						for _, w := range warns {
							fmt.Printf("  ! warning: %s: %s\n", w.Field, w.Message)
						}
					}
					os.Exit(0)
				} else {
					fmt.Printf("✗ %s invalid:\n", id)
					for _, e := range errs {
						fmt.Printf("  - %s: %s\n", e.Field, e.Message)
					}
					if showWarns && len(warns) > 0 {
						for _, w := range warns {
							fmt.Printf("  ! warning: %s: %s\n", w.Field, w.Message)
						}
					}
					os.Exit(1)
				}
			}
		} else {
			allErrs, allWarns, err := s.ValidateAll(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(2)
			}

			if format == "json" {
				// This is a bit complex for JSON because we don't have a list of all IDs easily here
				// But we can just report what we found
				printJSON(map[string]interface{}{
					"errors":   allErrs,
					"warnings": allWarns,
				})
			} else {
				invalidCount := len(allErrs)
				// We'll need a way to list all IDs to show "✓" for valid ones.
				// For now, let's just show the errors if any.
				if invalidCount == 0 {
					fmt.Println("All tickets valid.")
					os.Exit(0)
				} else {
					for id, errs := range allErrs {
						fmt.Printf("✗ %s invalid:\n", id)
						for _, e := range errs {
							fmt.Printf("  - %s: %s\n", e.Field, e.Message)
						}
					}
					fmt.Printf("\nFound %d invalid tickets.\n", invalidCount)
					os.Exit(1)
				}
			}
		}
	},
}

func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func init() {
	validateCmd.Flags().BoolVarP(&showWarns, "warn", "w", false, "show warnings")
	rootCmd.AddCommand(validateCmd)
}
