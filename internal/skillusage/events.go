package skillusage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
)

const (
	SchemaVersionV1 = "docket.skill_usage/v1"

	SourceCLI = "cli"
	SourceMCP = "mcp"
)

var Now = func() time.Time {
	return time.Now().UTC()
}

type Event struct {
	Version          string `json:"version"`
	InvokedAt        string `json:"invoked_at"`
	SkillID          string `json:"skill_id"`
	Source           string `json:"source"`
	TicketID         string `json:"ticket_id,omitempty"`
	Intent           string `json:"intent,omitempty"`
	Command          string `json:"command,omitempty"`
	MetadataChecksum string `json:"metadata_checksum,omitempty"`
}

type BucketSize string

const (
	BucketAuto BucketSize = "auto"
	BucketDay  BucketSize = "day"
	BucketWeek BucketSize = "week"
)

type SkillCount struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type Bucket struct {
	Start   string         `json:"start"`
	End     string         `json:"end"`
	Total   int            `json:"total"`
	Skills  map[string]int `json:"skills"`
	Sources map[string]int `json:"sources,omitempty"`
}

type Audit struct {
	SkillFilter      string         `json:"skill_filter,omitempty"`
	TotalInvocations int            `json:"total_invocations"`
	BucketSize       BucketSize     `json:"bucket_size"`
	From             string         `json:"from,omitempty"`
	To               string         `json:"to,omitempty"`
	Skills           []SkillCount   `json:"skills"`
	Sources          map[string]int `json:"sources,omitempty"`
	Timeline         []Bucket       `json:"timeline"`
}

func LogPath(repoRoot string) string {
	return artifacts.WriteRepoPath(repoRoot, artifacts.RepoSkillUsageEvents)
}

func Append(repoRoot string, event Event) error {
	if strings.TrimSpace(event.Version) == "" {
		event.Version = SchemaVersionV1
	}
	if strings.TrimSpace(event.InvokedAt) == "" {
		event.InvokedAt = Now().UTC().Format(time.RFC3339Nano)
	}
	event.SkillID = strings.TrimSpace(event.SkillID)
	event.Source = strings.TrimSpace(strings.ToLower(event.Source))
	event.TicketID = strings.TrimSpace(event.TicketID)
	event.Intent = strings.TrimSpace(event.Intent)
	event.Command = strings.TrimSpace(event.Command)
	event.MetadataChecksum = strings.TrimSpace(event.MetadataChecksum)

	if err := validateEvent(event); err != nil {
		return err
	}

	path := LogPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func Load(repoRoot string) ([]Event, error) {
	path := artifacts.ReadRepoPath(repoRoot, artifacts.RepoSkillUsageEvents)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, err
		}
		if err := validateEvent(ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].InvokedAt < events[j].InvokedAt
	})
	return events, nil
}

func BuildAudit(events []Event, filterSkillID string, requested BucketSize) (Audit, error) {
	filter := strings.ToLower(strings.TrimSpace(filterSkillID))
	filtered := make([]Event, 0, len(events))
	for _, ev := range events {
		if filter != "" && strings.ToLower(strings.TrimSpace(ev.SkillID)) != filter {
			continue
		}
		filtered = append(filtered, ev)
	}

	size, err := resolveBucketSize(filtered, requested)
	if err != nil {
		return Audit{}, err
	}
	out := Audit{
		SkillFilter: strings.TrimSpace(filterSkillID),
		BucketSize:  size,
		Sources:     map[string]int{},
		Timeline:    []Bucket{},
	}
	if len(filtered) == 0 {
		return out, nil
	}

	skillCounts := map[string]int{}
	sourceCounts := map[string]int{}
	buckets := map[string]*Bucket{}
	var starts []time.Time
	var minTime time.Time
	var maxTime time.Time
	for i, ev := range filtered {
		at, err := time.Parse(time.RFC3339Nano, ev.InvokedAt)
		if err != nil {
			return Audit{}, fmt.Errorf("parse invoked_at for %s: %w", ev.SkillID, err)
		}
		at = at.UTC()
		if i == 0 || at.Before(minTime) {
			minTime = at
		}
		if i == 0 || at.After(maxTime) {
			maxTime = at
		}
		start := bucketStart(at, size)
		key := start.Format("2006-01-02")
		if _, ok := buckets[key]; !ok {
			end := bucketEnd(start, size)
			buckets[key] = &Bucket{
				Start:   key,
				End:     end.Format("2006-01-02"),
				Skills:  map[string]int{},
				Sources: map[string]int{},
			}
			starts = append(starts, start)
		}
		bucket := buckets[key]
		bucket.Total++
		bucket.Skills[ev.SkillID]++
		bucket.Sources[ev.Source]++
		skillCounts[ev.SkillID]++
		sourceCounts[ev.Source]++
	}

	sort.SliceStable(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	for _, start := range starts {
		out.Timeline = append(out.Timeline, *buckets[start.Format("2006-01-02")])
	}

	skills := make([]SkillCount, 0, len(skillCounts))
	for id, count := range skillCounts {
		skills = append(skills, SkillCount{ID: id, Count: count})
	}
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Count == skills[j].Count {
			return skills[i].ID < skills[j].ID
		}
		return skills[i].Count > skills[j].Count
	})

	out.TotalInvocations = len(filtered)
	out.From = minTime.Format("2006-01-02")
	out.To = maxTime.Format("2006-01-02")
	out.Skills = skills
	out.Sources = sourceCounts
	return out, nil
}

func validateEvent(event Event) error {
	if strings.TrimSpace(event.Version) != SchemaVersionV1 {
		return fmt.Errorf("unsupported skill usage event version %q", event.Version)
	}
	if strings.TrimSpace(event.SkillID) == "" {
		return fmt.Errorf("skill_id is required")
	}
	switch strings.TrimSpace(strings.ToLower(event.Source)) {
	case SourceCLI, SourceMCP:
	default:
		return fmt.Errorf("source must be %q or %q", SourceCLI, SourceMCP)
	}
	if _, err := time.Parse(time.RFC3339Nano, event.InvokedAt); err != nil {
		return fmt.Errorf("invoked_at must be RFC3339: %w", err)
	}
	return nil
}

func resolveBucketSize(events []Event, requested BucketSize) (BucketSize, error) {
	switch requested {
	case "", BucketAuto:
	case BucketDay, BucketWeek:
		return requested, nil
	default:
		return "", fmt.Errorf("bucket must be auto, day, or week")
	}
	if len(events) < 2 {
		return BucketDay, nil
	}
	first, err := time.Parse(time.RFC3339Nano, events[0].InvokedAt)
	if err != nil {
		return "", err
	}
	last, err := time.Parse(time.RFC3339Nano, events[len(events)-1].InvokedAt)
	if err != nil {
		return "", err
	}
	if last.Sub(first) > 45*24*time.Hour {
		return BucketWeek, nil
	}
	return BucketDay, nil
}

func bucketStart(at time.Time, size BucketSize) time.Time {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	if size != BucketWeek {
		return day
	}
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func bucketEnd(start time.Time, size BucketSize) time.Time {
	if size == BucketWeek {
		return start.AddDate(0, 0, 6)
	}
	return start
}
