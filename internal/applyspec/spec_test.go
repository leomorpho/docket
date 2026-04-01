package applyspec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTicketSpecValid(t *testing.T) {
	raw := []byte(`{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Add apply schema",
    "description": "Define spec schema and validator for transactional apply commands.",
    "priority": 1,
    "state": "backlog",
    "labels": ["feature"],
    "ac": ["unit tests", "integration tests"]
  }
}`)

	spec, report, err := ParseTicketSpec(raw)
	if err != nil {
		t.Fatalf("ParseTicketSpec returned error: %v", err)
	}
	if !report.Valid() {
		t.Fatalf("expected valid report, got %#v", report.Errors)
	}
	if report.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("expected schema version %q, got %q", SchemaVersionV1, report.SchemaVersion)
	}
	if spec.Operation != OperationCreate {
		t.Fatalf("expected operation %q, got %q", OperationCreate, spec.Operation)
	}
	if spec.Ticket.Title != "Add apply schema" {
		t.Fatalf("unexpected title %q", spec.Ticket.Title)
	}
}

func TestParseTicketSpecValidationErrors(t *testing.T) {
	raw := []byte(`{
  "version": "docket.apply/v2",
  "operation": "create",
  "ticket": {
    "title": 7,
    "description": "",
    "priority": 0,
    "state": "invalid",
    "labels": ["feature", ""],
    "blocked_by": ["BAD"],
    "ac": ["ok", ""]
  }
}`)

	_, report, err := ParseTicketSpec(raw)
	if err != nil {
		t.Fatalf("ParseTicketSpec returned error: %v", err)
	}
	if report.Valid() {
		t.Fatal("expected validation errors")
	}

	assertHasError(t, report, "version", CodeUnsupportedVersion)
	assertHasError(t, report, "ticket.title", CodeTypeMismatch)
	assertHasError(t, report, "ticket.description", CodeRequired)
	assertHasError(t, report, "ticket.priority", CodeInvalidValue)
	assertHasError(t, report, "ticket.labels[1]", CodeRequired)
	assertHasError(t, report, "ticket.blocked_by[0]", CodeInvalidRelation)
	assertHasError(t, report, "ticket.ac[1]", CodeRequired)
}

func TestParseBacklogSpecInvalidRelations(t *testing.T) {
	raw := []byte(`{
  "version": "docket.apply/v1",
  "tickets": [
    {
      "ref": "epic",
      "title": "Epic",
      "description": "Top level epic."
    },
    {
      "ref": "child",
      "title": "Child",
      "description": "Child item.",
      "parent_ref": "missing-parent",
      "blocked_by": ["missing-ref", "TKT-009"]
    },
    {
      "ref": "child",
      "title": "Duplicate ref",
      "description": "Should fail."
    }
  ]
}`)

	spec, report, err := ParseBacklogSpec(raw)
	if err != nil {
		t.Fatalf("ParseBacklogSpec returned error: %v", err)
	}
	if report.Valid() {
		t.Fatal("expected validation errors")
	}

	if len(spec.Tickets) != 3 {
		t.Fatalf("expected 3 parsed tickets, got %d", len(spec.Tickets))
	}
	assertHasError(t, report, "tickets[1].parent_ref", CodeInvalidRelation)
	assertHasError(t, report, "tickets[1].blocked_by[0]", CodeInvalidRelation)
	assertHasError(t, report, "tickets[2].ref", CodeDuplicate)
}

func TestParseTicketSpecWithStatesAcceptsCustomWorkflowNames(t *testing.T) {
	raw := []byte(`{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Custom workflow",
    "description": "Uses renamed states from config.",
    "state": "building"
  }
}`)

	spec, report, err := ParseTicketSpecWithStates(raw, map[string]struct{}{
		"queued":   {},
		"building": {},
		"qa":       {},
		"shipped":  {},
	})
	if err != nil {
		t.Fatalf("ParseTicketSpecWithStates returned error: %v", err)
	}
	if !report.Valid() {
		t.Fatalf("expected valid report, got %#v", report.Errors)
	}
	if spec.Ticket.State != "building" {
		t.Fatalf("state = %q, want building", spec.Ticket.State)
	}
}

func TestParseBacklogSpecWithStatesRejectsUnknownCustomStateDeterministically(t *testing.T) {
	raw := []byte(`{
  "version": "docket.apply/v1",
  "tickets": [
    {
      "ref": "epic",
      "title": "Epic",
      "description": "Root",
      "state": "reviewing"
    }
  ]
}`)

	_, report, err := ParseBacklogSpecWithStates(raw, map[string]struct{}{
		"queued":   {},
		"building": {},
		"qa":       {},
		"shipped":  {},
	})
	if err != nil {
		t.Fatalf("ParseBacklogSpecWithStates returned error: %v", err)
	}
	if report.Valid() {
		t.Fatal("expected validation errors")
	}
	assertHasError(t, report, "tickets[0].state", CodeInvalidValue)
	if got := report.Errors[0].Message; got != "must be one of building,qa,queued,shipped" {
		t.Fatalf("unexpected state error message %q", got)
	}
}

func TestLoadSpecFilesIntegration(t *testing.T) {
	tmp := t.TempDir()
	goodPath := filepath.Join(tmp, "ticket.good.json")
	badPath := filepath.Join(tmp, "backlog.bad.json")

	good := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Create validator",
    "description": "Implement schema validator with deterministic output.",
    "labels": ["feature"],
    "ac": ["unit", "integration"]
  }
}`
	bad := `{
  "version": "docket.apply/v1",
  "tickets": [
    {
      "ref": "root",
      "title": "Root",
      "description": "Root ticket"
    },
    {
      "ref": "child",
      "title": "Child",
      "description": "Child ticket",
      "parent_ref": "ghost"
    }
  ]
}`

	if err := os.WriteFile(goodPath, []byte(good), 0o644); err != nil {
		t.Fatalf("write good spec: %v", err)
	}
	if err := os.WriteFile(badPath, []byte(bad), 0o644); err != nil {
		t.Fatalf("write bad spec: %v", err)
	}

	_, goodReport, err := LoadTicketSpecFile(goodPath)
	if err != nil {
		t.Fatalf("LoadTicketSpecFile returned error: %v", err)
	}
	if !goodReport.Valid() {
		t.Fatalf("expected valid report for good spec, got %#v", goodReport.Errors)
	}

	_, badReport, err := LoadBacklogSpecFile(badPath)
	if err != nil {
		t.Fatalf("LoadBacklogSpecFile returned error: %v", err)
	}
	if badReport.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("expected schema version %q, got %q", SchemaVersionV1, badReport.SchemaVersion)
	}
	assertHasError(t, badReport, "tickets[1].parent_ref", CodeInvalidRelation)

	encoded, err := json.Marshal(badReport)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if !strings.Contains(string(encoded), `"schema_version":"docket.apply/v1"`) {
		t.Fatalf("expected schema_version in structured output: %s", string(encoded))
	}
	if !strings.Contains(string(encoded), `"path":"tickets[1].parent_ref"`) {
		t.Fatalf("expected field-level error path in structured output: %s", string(encoded))
	}
}

func TestEmbeddedSchemasIncludeVersionConst(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "ticket", data: TicketSchemaJSON()},
		{name: "backlog", data: BacklogSchemaJSON()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(tc.data, &doc); err != nil {
				t.Fatalf("failed to parse embedded schema: %v", err)
			}

			props, ok := doc["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema missing properties: %#v", doc)
			}
			version, ok := props["version"].(map[string]any)
			if !ok {
				t.Fatalf("schema missing version property: %#v", props)
			}
			if version["const"] != SchemaVersionV1 {
				t.Fatalf("expected version const %q, got %#v", SchemaVersionV1, version["const"])
			}
		})
	}
}

func TestEmbeddedSchemasDoNotHardcodeWorkflowStateEnum(t *testing.T) {
	cases := []struct {
		name      string
		data      []byte
		fieldPath []string
	}{
		{name: "ticket", data: TicketSchemaJSON(), fieldPath: []string{"properties", "ticket", "properties", "state"}},
		{name: "backlog", data: BacklogSchemaJSON(), fieldPath: []string{"properties", "tickets", "items", "properties", "state"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(tc.data, &doc); err != nil {
				t.Fatalf("failed to parse embedded schema: %v", err)
			}
			node := any(doc)
			for _, key := range tc.fieldPath {
				obj, ok := node.(map[string]any)
				if !ok {
					t.Fatalf("schema path %v missing object at %q", tc.fieldPath, key)
				}
				node = obj[key]
			}
			stateSchema, ok := node.(map[string]any)
			if !ok {
				t.Fatalf("state schema at path %v is not an object: %#v", tc.fieldPath, node)
			}
			if _, ok := stateSchema["enum"]; ok {
				t.Fatalf("state schema unexpectedly contains fixed enum: %#v", stateSchema["enum"])
			}
		})
	}
}

func assertHasError(t *testing.T, report ValidationReport, path, code string) {
	t.Helper()
	for _, e := range report.Errors {
		if e.Path == path && e.Code == code {
			return
		}
	}
	t.Fatalf("expected error path=%q code=%q, got %#v", path, code, report.Errors)
}
