package skillusage

import (
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
)

func TestAppendLoadAndBuildAudit(t *testing.T) {
	repo := t.TempDir()

	originalNow := Now
	defer func() { Now = originalNow }()

	stamps := []time.Time{
		time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC),
	}
	idx := 0
	Now = func() time.Time {
		if idx >= len(stamps) {
			return stamps[len(stamps)-1]
		}
		cur := stamps[idx]
		idx++
		return cur
	}

	for _, ev := range []Event{
		{SkillID: "learning-replay", Source: SourceCLI, TicketID: "TKT-100"},
		{SkillID: "wrap-up-readiness", Source: SourceMCP, TicketID: "TKT-100"},
		{SkillID: "learning-replay", Source: SourceCLI, TicketID: "TKT-200"},
	} {
		if err := Append(repo, ev); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	if got, want := LogPath(repo), artifacts.WriteRepoPath(repo, artifacts.RepoSkillUsageEvents); got != want {
		t.Fatalf("log path = %s, want %s", got, want)
	}

	events, err := Load(repo)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	audit, err := BuildAudit(events, "", BucketAuto)
	if err != nil {
		t.Fatalf("build audit failed: %v", err)
	}
	if audit.BucketSize != BucketWeek {
		t.Fatalf("expected auto bucket to choose week, got %s", audit.BucketSize)
	}
	if audit.TotalInvocations != 3 {
		t.Fatalf("expected 3 invocations, got %d", audit.TotalInvocations)
	}
	if audit.Sources[SourceCLI] != 2 || audit.Sources[SourceMCP] != 1 {
		t.Fatalf("unexpected source counts: %#v", audit.Sources)
	}
	if len(audit.Skills) != 2 || audit.Skills[0].ID != "learning-replay" || audit.Skills[0].Count != 2 {
		t.Fatalf("unexpected skill counts: %#v", audit.Skills)
	}
	if len(audit.Timeline) != 2 {
		t.Fatalf("expected 2 timeline buckets, got %#v", audit.Timeline)
	}

	filtered, err := BuildAudit(events, "wrap-up-readiness", BucketDay)
	if err != nil {
		t.Fatalf("build filtered audit failed: %v", err)
	}
	if filtered.TotalInvocations != 1 {
		t.Fatalf("expected filtered count 1, got %d", filtered.TotalInvocations)
	}
	if len(filtered.Skills) != 1 || filtered.Skills[0].ID != "wrap-up-readiness" {
		t.Fatalf("unexpected filtered skills: %#v", filtered.Skills)
	}
	if filtered.BucketSize != BucketDay {
		t.Fatalf("expected explicit day bucket, got %s", filtered.BucketSize)
	}
}

func TestBuildAuditRejectsInvalidBucket(t *testing.T) {
	if _, err := BuildAudit(nil, "", BucketSize("month")); err == nil {
		t.Fatal("expected invalid bucket error")
	}
}
