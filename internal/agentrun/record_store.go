package agentrun

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/artifacts"
)

func RunRecordPath(repoRoot, ticketID string) string {
	return artifacts.WriteRepoPath(repoRoot, artifacts.RepoAgentRunsDir, ticketID+".json")
}

func WriteRunRecord(repoRoot string, record RunRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	path := RunRecordPath(repoRoot, record.TicketID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func LoadRunRecord(repoRoot, ticketID string) (RunRecord, bool, error) {
	data, err := os.ReadFile(RunRecordPath(repoRoot, ticketID))
	if err != nil {
		if os.IsNotExist(err) {
			return RunRecord{}, false, nil
		}
		return RunRecord{}, false, err
	}
	var record RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RunRecord{}, false, err
	}
	return record, true, nil
}
