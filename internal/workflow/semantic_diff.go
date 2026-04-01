package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type workflowPolicyView struct {
	States    map[string][]string `json:"states"`
	Semantics *workflowSemantics  `json:"semantics,omitempty"`
}

type workflowSemantics struct {
	Review           []string `json:"review,omitempty"`
	Verification     []string `json:"verification,omitempty"`
	Closure          []string `json:"closure,omitempty"`
	HumanOnlyClosure bool     `json:"human_only_closure"`
}

type TransitionChange struct {
	State   string   `json:"state"`
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

type SemanticPolicyDiff struct {
	AddedStates        []string           `json:"added_states,omitempty"`
	RemovedStates      []string           `json:"removed_states,omitempty"`
	TransitionChanges  []TransitionChange `json:"transition_changes,omitempty"`
	SemanticDiffLines  []string           `json:"semantic_diff_lines,omitempty"`
}

func DiffWorkflowPolicy(beforePolicy, afterPolicy json.RawMessage) (SemanticPolicyDiff, error) {
	var before workflowPolicyView
	if err := json.Unmarshal(beforePolicy, &before); err != nil {
		return SemanticPolicyDiff{}, fmt.Errorf("parse before policy: %w", err)
	}
	var after workflowPolicyView
	if err := json.Unmarshal(afterPolicy, &after); err != nil {
		return SemanticPolicyDiff{}, fmt.Errorf("parse after policy: %w", err)
	}

	diff := SemanticPolicyDiff{}
	allStates := map[string]struct{}{}
	for state := range before.States {
		allStates[state] = struct{}{}
	}
	for state := range after.States {
		allStates[state] = struct{}{}
	}

	states := make([]string, 0, len(allStates))
	for state := range allStates {
		states = append(states, state)
	}
	sort.Strings(states)

	for _, state := range states {
		_, inBefore := before.States[state]
		_, inAfter := after.States[state]
		switch {
		case inBefore && !inAfter:
			diff.RemovedStates = append(diff.RemovedStates, state)
		case !inBefore && inAfter:
			diff.AddedStates = append(diff.AddedStates, state)
		}

		beforeSet := toSet(before.States[state])
		afterSet := toSet(after.States[state])
		added := difference(afterSet, beforeSet)
		removed := difference(beforeSet, afterSet)
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		diff.TransitionChanges = append(diff.TransitionChanges, TransitionChange{
			State:   state,
			Added:   added,
			Removed: removed,
		})
	}

	diff.SemanticDiffLines = append(diff.SemanticDiffLines, semanticFieldChanges(before.Semantics, after.Semantics)...)
	return diff, nil
}

func RenderWorkflowPolicyDiffHuman(diff SemanticPolicyDiff) string {
	lines := []string{}
	if len(diff.AddedStates) > 0 {
		lines = append(lines, fmt.Sprintf("- Added states: %s", strings.Join(diff.AddedStates, ", ")))
	}
	if len(diff.RemovedStates) > 0 {
		lines = append(lines, fmt.Sprintf("- Removed states: %s", strings.Join(diff.RemovedStates, ", ")))
	}
	for _, change := range diff.TransitionChanges {
		parts := []string{}
		if len(change.Added) > 0 {
			parts = append(parts, "+"+strings.Join(change.Added, ", +"))
		}
		if len(change.Removed) > 0 {
			parts = append(parts, "-"+strings.Join(change.Removed, ", -"))
		}
		lines = append(lines, fmt.Sprintf("- %s transitions: %s", change.State, strings.Join(parts, " ")))
	}
	for _, line := range diff.SemanticDiffLines {
		lines = append(lines, "- "+line)
	}
	if len(lines) == 0 {
		return "- No semantic workflow changes detected."
	}
	return strings.Join(lines, "\n")
}

func semanticFieldChanges(before, after *workflowSemantics) []string {
	lines := []string{}
	switch {
	case before == nil && after == nil:
		return lines
	case before == nil && after != nil:
		lines = append(lines, "semantics block added")
		return lines
	case before != nil && after == nil:
		lines = append(lines, "semantics block removed")
		return lines
	}

	compareSlice := func(name string, oldVals, newVals []string) {
		oldSet := toSet(oldVals)
		newSet := toSet(newVals)
		added := difference(newSet, oldSet)
		removed := difference(oldSet, newSet)
		if len(added) == 0 && len(removed) == 0 {
			return
		}
		parts := []string{}
		if len(added) > 0 {
			parts = append(parts, "added="+strings.Join(added, ","))
		}
		if len(removed) > 0 {
			parts = append(parts, "removed="+strings.Join(removed, ","))
		}
		lines = append(lines, fmt.Sprintf("semantics.%s changed (%s)", name, strings.Join(parts, " ")))
	}
	compareSlice("review", before.Review, after.Review)
	compareSlice("verification", before.Verification, after.Verification)
	compareSlice("closure", before.Closure, after.Closure)
	if before.HumanOnlyClosure != after.HumanOnlyClosure {
		lines = append(lines, fmt.Sprintf("semantics.human_only_closure changed (%t -> %t)", before.HumanOnlyClosure, after.HumanOnlyClosure))
	}
	return lines
}

func toSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func difference(a, b map[string]struct{}) []string {
	out := []string{}
	for k := range a {
		if _, ok := b[k]; ok {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
