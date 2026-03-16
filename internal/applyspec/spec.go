package applyspec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersionV1 = "docket.apply/v1"

	OperationCreate = "create"
	OperationUpdate = "update"
	OperationUpsert = "upsert"

	CodeRequired           = "required"
	CodeTypeMismatch       = "type_mismatch"
	CodeInvalidValue       = "invalid_value"
	CodeInvalidRelation    = "invalid_relation"
	CodeUnsupportedVersion = "unsupported_version"
	CodeDuplicate          = "duplicate"
)

var (
	ticketIDPattern = regexp.MustCompile(`^TKT-\d+$`)
	refPattern      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

	allowedStates = map[string]struct{}{
		"backlog":     {},
		"todo":        {},
		"in-progress": {},
		"in-review":   {},
		"done":        {},
		"archived":    {},
	}
)

// Versioning defines supported schema versions and upgrade policy.
type Versioning struct {
	Current     string   `json:"current"`
	Supported   []string `json:"supported"`
	UpgradeNote string   `json:"upgrade_note"`
}

// ValidationError is a structured, field-level validator error.
type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationReport includes schema version and deterministic structured errors.
type ValidationReport struct {
	SchemaVersion string            `json:"schema_version"`
	Errors        []ValidationError `json:"errors,omitempty"`
}

func (r ValidationReport) Valid() bool {
	return len(r.Errors) == 0
}

// TicketApplySpec is the canonical single-ticket apply schema.
type TicketApplySpec struct {
	Version   string     `json:"version"`
	Operation string     `json:"operation,omitempty"`
	Ticket    TicketSpec `json:"ticket"`
}

// TicketSpec captures mutable ticket fields for ticket apply payloads.
type TicketSpec struct {
	ID          string   `json:"id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	State       string   `json:"state,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	AC          []string `json:"ac,omitempty"`
}

// BacklogApplySpec is the canonical multi-ticket apply schema.
type BacklogApplySpec struct {
	Version string              `json:"version"`
	Tickets []BacklogTicketSpec `json:"tickets"`
}

// BacklogTicketSpec defines one ticket entry within a backlog apply payload.
type BacklogTicketSpec struct {
	Ref         string   `json:"ref,omitempty"`
	ID          string   `json:"id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	State       string   `json:"state,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	ParentRef   string   `json:"parent_ref,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	AC          []string `json:"ac,omitempty"`
}

// SchemaVersioning documents compatibility rules for future evolution.
func SchemaVersioning() Versioning {
	return Versioning{
		Current:     SchemaVersionV1,
		Supported:   []string{SchemaVersionV1},
		UpgradeNote: "New schema revisions must bump the version and remain backward-compatible when feasible.",
	}
}

func LoadTicketSpecFile(path string) (TicketApplySpec, ValidationReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TicketApplySpec{}, ValidationReport{}, err
	}
	return ParseTicketSpec(data)
}

func LoadBacklogSpecFile(path string) (BacklogApplySpec, ValidationReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BacklogApplySpec{}, ValidationReport{}, err
	}
	return ParseBacklogSpec(data)
}

func ParseTicketSpec(data []byte) (TicketApplySpec, ValidationReport, error) {
	root, err := decodeRootObject(data)
	if err != nil {
		return TicketApplySpec{}, ValidationReport{}, err
	}
	v := &validator{}
	spec := TicketApplySpec{}
	spec.Version = parseVersion(root, v)
	report := ValidationReport{SchemaVersion: schemaVersionForReport(spec.Version)}

	spec.Operation = parseOperation(root, v)
	ticketRaw, ok := root["ticket"]
	if !ok {
		v.add("ticket", CodeRequired, "field is required")
	} else {
		obj, ok := ticketRaw.(map[string]any)
		if !ok {
			v.add("ticket", CodeTypeMismatch, "must be an object")
		} else {
			ticketSpec, present := parseTicketObject(obj, "ticket", false, v)
			spec.Ticket = TicketSpec{
				ID:          ticketSpec.ID,
				Title:       ticketSpec.Title,
				Description: ticketSpec.Description,
				Priority:    ticketSpec.Priority,
				State:       ticketSpec.State,
				Labels:      ticketSpec.Labels,
				Parent:      ticketSpec.Parent,
				BlockedBy:   ticketSpec.BlockedBy,
				AC:          ticketSpec.AC,
			}
			applyOperationRules(spec.Operation, ticketSpec, present, v)
		}
	}

	report.Errors = v.sortedErrors()
	return spec, report, nil
}

func ParseBacklogSpec(data []byte) (BacklogApplySpec, ValidationReport, error) {
	root, err := decodeRootObject(data)
	if err != nil {
		return BacklogApplySpec{}, ValidationReport{}, err
	}
	v := &validator{}
	spec := BacklogApplySpec{}
	spec.Version = parseVersion(root, v)
	report := ValidationReport{SchemaVersion: schemaVersionForReport(spec.Version)}

	ticketsRaw, ok := root["tickets"]
	if !ok {
		v.add("tickets", CodeRequired, "field is required")
		report.Errors = v.sortedErrors()
		return spec, report, nil
	}

	items, ok := ticketsRaw.([]any)
	if !ok {
		v.add("tickets", CodeTypeMismatch, "must be an array")
		report.Errors = v.sortedErrors()
		return spec, report, nil
	}
	if len(items) == 0 {
		v.add("tickets", CodeRequired, "must include at least one ticket")
	}

	spec.Tickets = make([]BacklogTicketSpec, 0, len(items))
	refs := make(map[string]int)
	for i, item := range items {
		path := fmt.Sprintf("tickets[%d]", i)
		obj, ok := item.(map[string]any)
		if !ok {
			v.add(path, CodeTypeMismatch, "must be an object")
			spec.Tickets = append(spec.Tickets, BacklogTicketSpec{})
			continue
		}
		ticketSpec, _ := parseTicketObject(obj, path, true, v)
		backlogTicket := BacklogTicketSpec{
			Ref:         ticketSpec.Ref,
			ID:          ticketSpec.ID,
			Title:       ticketSpec.Title,
			Description: ticketSpec.Description,
			Priority:    ticketSpec.Priority,
			State:       ticketSpec.State,
			Labels:      ticketSpec.Labels,
			Parent:      ticketSpec.Parent,
			ParentRef:   ticketSpec.ParentRef,
			BlockedBy:   ticketSpec.BlockedBy,
			AC:          ticketSpec.AC,
		}
		spec.Tickets = append(spec.Tickets, backlogTicket)

		if ticketSpec.Ref != "" {
			if prev, exists := refs[ticketSpec.Ref]; exists {
				v.add(path+".ref", CodeDuplicate, fmt.Sprintf("duplicate ref %q already defined at tickets[%d].ref", ticketSpec.Ref, prev))
			} else {
				refs[ticketSpec.Ref] = i
			}
		}
	}

	validateBacklogRelations(spec.Tickets, refs, v)
	report.Errors = v.sortedErrors()
	return spec, report, nil
}

type parsedTicket struct {
	Ref         string
	ParentRef   string
	ID          string
	Title       string
	Description string
	Priority    *int
	State       string
	Labels      []string
	Parent      string
	BlockedBy   []string
	AC          []string
}

type fieldPresence struct {
	ID          bool
	Title       bool
	Description bool
	Priority    bool
	State       bool
	Labels      bool
	Parent      bool
	BlockedBy   bool
	AC          bool
}

func parseTicketObject(obj map[string]any, path string, allowRefs bool, v *validator) (parsedTicket, fieldPresence) {
	out := parsedTicket{}
	present := fieldPresence{}

	if allowRefs {
		if val, ok := readOptionalString(obj, "ref", path+".ref", v); ok {
			out.Ref = val
			if val != "" && !refPattern.MatchString(val) {
				v.add(path+".ref", CodeInvalidValue, "must match ^[A-Za-z][A-Za-z0-9_.-]*$")
			}
		}
		if val, ok := readOptionalString(obj, "parent_ref", path+".parent_ref", v); ok {
			out.ParentRef = val
		}
	}

	if val, ok := readOptionalString(obj, "id", path+".id", v); ok {
		present.ID = true
		out.ID = val
		if val != "" && !ticketIDPattern.MatchString(val) {
			v.add(path+".id", CodeInvalidRelation, "must match TKT-<number>")
		}
	}

	if val, ok := readOptionalString(obj, "title", path+".title", v); ok {
		present.Title = true
		out.Title = val
	}

	if val, ok := readOptionalString(obj, "description", path+".description", v); ok {
		present.Description = true
		out.Description = val
	}

	if val, ok := readOptionalInt(obj, "priority", path+".priority", v); ok {
		present.Priority = true
		out.Priority = val
		if val != nil && *val <= 0 {
			v.add(path+".priority", CodeInvalidValue, "must be greater than 0")
		}
	}

	if val, ok := readOptionalString(obj, "state", path+".state", v); ok {
		present.State = true
		out.State = val
		if val != "" {
			if _, exists := allowedStates[val]; !exists {
				v.add(path+".state", CodeInvalidValue, "must be one of backlog,todo,in-progress,in-review,done,archived")
			}
		}
	}

	if val, ok := readOptionalStringSlice(obj, "labels", path+".labels", v); ok {
		present.Labels = true
		out.Labels = val
		seen := make(map[string]int)
		for i, label := range val {
			if label == "" {
				v.add(fmt.Sprintf("%s.labels[%d]", path, i), CodeRequired, "label cannot be empty")
				continue
			}
			key := strings.ToLower(label)
			if prev, exists := seen[key]; exists {
				v.add(fmt.Sprintf("%s.labels[%d]", path, i), CodeDuplicate, fmt.Sprintf("duplicate label also used at labels[%d]", prev))
				continue
			}
			seen[key] = i
		}
	}

	if val, ok := readOptionalString(obj, "parent", path+".parent", v); ok {
		present.Parent = true
		out.Parent = val
		if val != "" && !ticketIDPattern.MatchString(val) {
			v.add(path+".parent", CodeInvalidRelation, "must match TKT-<number>")
		}
	}

	if val, ok := readOptionalStringSlice(obj, "blocked_by", path+".blocked_by", v); ok {
		present.BlockedBy = true
		out.BlockedBy = val
		for i, dep := range val {
			if dep == "" {
				v.add(fmt.Sprintf("%s.blocked_by[%d]", path, i), CodeRequired, "blocked_by entry cannot be empty")
				continue
			}
			if allowRefs {
				if ticketIDPattern.MatchString(dep) || refPattern.MatchString(dep) {
					continue
				}
				v.add(fmt.Sprintf("%s.blocked_by[%d]", path, i), CodeInvalidRelation, "must reference a ticket ID or ticket ref")
				continue
			}
			if !ticketIDPattern.MatchString(dep) {
				v.add(fmt.Sprintf("%s.blocked_by[%d]", path, i), CodeInvalidRelation, "must match TKT-<number>")
			}
		}
	}

	if val, ok := readOptionalStringSlice(obj, "ac", path+".ac", v); ok {
		present.AC = true
		out.AC = val
		for i, entry := range val {
			if entry == "" {
				v.add(fmt.Sprintf("%s.ac[%d]", path, i), CodeRequired, "AC entry cannot be empty")
			}
		}
	}

	return out, present
}

func applyOperationRules(operation string, ticket parsedTicket, present fieldPresence, v *validator) {
	requiresCreateFields := ticket.ID == ""
	if operation == OperationCreate {
		requiresCreateFields = true
	}

	if operation == OperationUpdate {
		if ticket.ID == "" {
			v.add("ticket.id", CodeRequired, "id is required for update operation")
		}
		if !hasMutableFields(present) {
			v.add("ticket", CodeInvalidValue, "update operation requires at least one mutable field")
		}
	}

	if requiresCreateFields {
		if ticket.Title == "" {
			v.add("ticket.title", CodeRequired, "title is required when creating a ticket")
		}
		if ticket.Description == "" {
			v.add("ticket.description", CodeRequired, "description is required when creating a ticket")
		}
	}
}

func hasMutableFields(p fieldPresence) bool {
	return p.Title || p.Description || p.Priority || p.State || p.Labels || p.Parent || p.BlockedBy || p.AC
}

func validateBacklogRelations(tickets []BacklogTicketSpec, refs map[string]int, v *validator) {
	for i := range tickets {
		path := fmt.Sprintf("tickets[%d]", i)
		tk := tickets[i]

		if tk.Ref == "" && tk.ID == "" {
			v.add(path+".ref", CodeRequired, "ref is required when id is omitted")
		}
		if tk.ID == "" {
			if tk.Title == "" {
				v.add(path+".title", CodeRequired, "title is required when creating a ticket")
			}
			if tk.Description == "" {
				v.add(path+".description", CodeRequired, "description is required when creating a ticket")
			}
		}
		if tk.ParentRef != "" {
			if tk.Ref != "" && tk.ParentRef == tk.Ref {
				v.add(path+".parent_ref", CodeInvalidRelation, "parent_ref cannot reference itself")
			}
			if _, ok := refs[tk.ParentRef]; !ok {
				v.add(path+".parent_ref", CodeInvalidRelation, fmt.Sprintf("unknown ref %q", tk.ParentRef))
			}
		}
		if tk.Parent != "" && tk.ParentRef != "" {
			v.add(path+".parent_ref", CodeInvalidValue, "cannot set both parent and parent_ref")
		}

		for j, dep := range tk.BlockedBy {
			depPath := fmt.Sprintf("%s.blocked_by[%d]", path, j)
			if dep == "" {
				v.add(depPath, CodeRequired, "blocked_by entry cannot be empty")
				continue
			}
			if tk.Ref != "" && dep == tk.Ref {
				v.add(depPath, CodeInvalidRelation, "blocked_by cannot reference itself")
				continue
			}
			if ticketIDPattern.MatchString(dep) {
				continue
			}
			if _, ok := refs[dep]; !ok {
				v.add(depPath, CodeInvalidRelation, fmt.Sprintf("unknown ticket reference %q", dep))
			}
		}
	}
}

func parseVersion(root map[string]any, v *validator) string {
	value, ok := readOptionalString(root, "version", "version", v)
	if !ok {
		v.add("version", CodeRequired, "field is required")
		return ""
	}
	if value == "" {
		v.add("version", CodeRequired, "field is required")
		return ""
	}
	if value != SchemaVersionV1 {
		v.add("version", CodeUnsupportedVersion, fmt.Sprintf("unsupported version %q", value))
	}
	return value
}

func parseOperation(root map[string]any, v *validator) string {
	value, ok := readOptionalString(root, "operation", "operation", v)
	if !ok || value == "" {
		return OperationUpsert
	}
	switch value {
	case OperationCreate, OperationUpdate, OperationUpsert:
		return value
	default:
		v.add("operation", CodeInvalidValue, "must be one of create, update, upsert")
		return value
	}
}

func schemaVersionForReport(parsed string) string {
	if strings.TrimSpace(parsed) == "" {
		return SchemaVersionV1
	}
	return parsed
}

func decodeRootObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("spec must be a JSON object")
	}
	return obj, nil
}

func readOptionalString(obj map[string]any, key, path string, v *validator) (string, bool) {
	raw, ok := obj[key]
	if !ok {
		return "", false
	}
	str, ok := raw.(string)
	if !ok {
		v.add(path, CodeTypeMismatch, "must be a string")
		return "", true
	}
	return strings.TrimSpace(str), true
}

func readOptionalStringSlice(obj map[string]any, key, path string, v *validator) ([]string, bool) {
	raw, ok := obj[key]
	if !ok {
		return nil, false
	}
	arr, ok := raw.([]any)
	if !ok {
		v.add(path, CodeTypeMismatch, "must be an array of strings")
		return nil, true
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		str, ok := item.(string)
		if !ok {
			v.add(fmt.Sprintf("%s[%d]", path, i), CodeTypeMismatch, "must be a string")
			out = append(out, "")
			continue
		}
		out = append(out, strings.TrimSpace(str))
	}
	return out, true
}

func readOptionalInt(obj map[string]any, key, path string, v *validator) (*int, bool) {
	raw, ok := obj[key]
	if !ok {
		return nil, false
	}
	num, ok := raw.(json.Number)
	if !ok {
		v.add(path, CodeTypeMismatch, "must be an integer")
		return nil, true
	}
	ival, err := num.Int64()
	if err != nil {
		v.add(path, CodeTypeMismatch, "must be an integer")
		return nil, true
	}
	val := int(ival)
	return &val, true
}

type validator struct {
	errors []ValidationError
}

func (v *validator) add(path, code, message string) {
	v.errors = append(v.errors, ValidationError{Path: path, Code: code, Message: message})
}

func (v *validator) sortedErrors() []ValidationError {
	out := make([]ValidationError, len(v.errors))
	copy(out, v.errors)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
}
