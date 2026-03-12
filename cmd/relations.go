package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type relationEntry struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

type relationState struct {
	Relations []relationEntry `json:"relations"`
}

func relationsPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "relations.json")
}

func loadRelations(repoRoot string) (relationState, error) {
	var st relationState
	data, err := os.ReadFile(relationsPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveRelations(repoRoot string, st relationState) error {
	if err := os.MkdirAll(filepath.Dir(relationsPath(repoRoot)), 0o755); err != nil {
		return err
	}
	sort.Slice(st.Relations, func(i, j int) bool {
		if st.Relations[i].From == st.Relations[j].From {
			if st.Relations[i].To == st.Relations[j].To {
				return st.Relations[i].Relation < st.Relations[j].Relation
			}
			return st.Relations[i].To < st.Relations[j].To
		}
		return st.Relations[i].From < st.Relations[j].From
	})
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(relationsPath(repoRoot), append(data, '\n'), 0o644)
}

func upsertRelation(repoRoot string, r relationEntry) error {
	st, err := loadRelations(repoRoot)
	if err != nil {
		return err
	}
	next := make([]relationEntry, 0, len(st.Relations)+1)
	replaced := false
	for _, ex := range st.Relations {
		if ex.From == r.From && ex.To == r.To {
			ex.Relation = r.Relation
			next = append(next, ex)
			replaced = true
			continue
		}
		next = append(next, ex)
	}
	if !replaced {
		next = append(next, r)
	}
	st.Relations = next
	return saveRelations(repoRoot, st)
}
