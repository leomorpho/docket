package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type doctorSummary struct {
	Pass int `json:"pass"`
	Fail int `json:"fail"`
}

type doctorCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation,omitempty"`
}

type doctorReport struct {
	Adapter string        `json:"adapter"`
	Summary doctorSummary `json:"summary"`
	Checks  []doctorCheck `json:"checks"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run setup and integration health checks for MCP, skills, hooks, and capabilities contract",
	RunE: func(cmd *cobra.Command, args []string) error {
		report := buildDoctorReport(repo)
		if format == "json" {
			printJSON(cmd, report)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Setup and integration health report (adapter: %s)\n", report.Adapter)
		for _, chk := range report.Checks {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %-8s %s\n", chk.Status, chk.Name, chk.Detail)
			if chk.Status == "FAIL" && chk.Remediation != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Fix: %s\n", chk.Remediation)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d pass, %d fail\n", report.Summary.Pass, report.Summary.Fail)
		return nil
	},
}

func buildDoctorReport(repoRoot string) doctorReport {
	digest := buildStartCapabilityDigest(repoRoot)
	report := doctorReport{
		Adapter: digest.Adapter,
		Checks: []doctorCheck{
			newReadinessCheck("mcp", digest.Readiness.MCP == "ready", "MCP wiring detected.", "MCP wiring not detected.", digest.Adapter),
			newReadinessCheck("skills", digest.Readiness.Skills == "ready", "Skill integration detected.", "Skill integration not detected.", digest.Adapter),
			newReadinessCheck("hooks", digest.Readiness.Hooks == "ready", "Git hook wiring is healthy.", "Git hook wiring is missing or stale.", digest.Adapter),
			buildContractCheck(repoRoot, digest.Adapter),
			buildQueueInvariantCheck(repoRoot),
		},
	}
	for _, chk := range report.Checks {
		if chk.Status == "PASS" {
			report.Summary.Pass++
		} else {
			report.Summary.Fail++
		}
	}
	return report
}

func buildQueueInvariantCheck(repoRoot string) doctorCheck {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return doctorCheck{
			Name:        "queue_invariant",
			Status:      "FAIL",
			Detail:      "Unable to load workflow config for queue invariant checks.",
			Remediation: "Run `docket init` or fix .docket/config.json.",
		}
	}
	s := local.New(repoRoot)
	if err := enforceStartableLeafInvariant(context.Background(), s, cfg, false); err != nil {
		return doctorCheck{
			Name:        "queue_invariant",
			Status:      "FAIL",
			Detail:      err.Error(),
			Remediation: "Run `docket queue heal` or unblock a startable leaf ticket.",
		}
	}
	return doctorCheck{
		Name:   "queue_invariant",
		Status: "PASS",
		Detail: "At least one unblocked startable leaf ticket is available.",
	}
}

func newReadinessCheck(name string, ok bool, passDetail, failDetail, adapterID string) doctorCheck {
	if ok {
		return doctorCheck{
			Name:   name,
			Status: "PASS",
			Detail: passDetail,
		}
	}
	return doctorCheck{
		Name:        name,
		Status:      "FAIL",
		Detail:      failDetail,
		Remediation: bootstrapFixCommand(adapterID),
	}
}

func buildContractCheck(repoRoot, adapterID string) doctorCheck {
	contractPath := artifacts.ReadRepoPath(repoRoot, artifacts.RepoRuntimeCapabilities)
	contract, err := capabilities.LoadRuntimeContract(repoRoot)
	if err == nil {
		return doctorCheck{
			Name:   "contract",
			Status: "PASS",
			Detail: fmt.Sprintf("Capabilities contract v%d loaded (%s).", contract.Version, contract.Hash),
		}
	}
	detail := "Capabilities contract is missing or invalid."
	if _, statErr := os.Stat(contractPath); statErr == nil {
		detail = "Capabilities contract exists but failed validation."
	}
	return doctorCheck{
		Name:        "contract",
		Status:      "FAIL",
		Detail:      detail,
		Remediation: fmt.Sprintf("%s and ensure %s is generated.", bootstrapFixCommand(adapterID), capabilities.DefaultRuntimeContractPath),
	}
}

func bootstrapFixCommand(adapterID string) string {
	if adapterID != "" && adapterID != "unsupported" && adapterID != "auto-detect" && adapterID != "unknown" {
		return fmt.Sprintf("Run `docket bootstrap --adapter %s`.", adapterID)
	}
	return "Run `docket bootstrap`."
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
