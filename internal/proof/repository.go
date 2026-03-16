package proof

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ticketIDPattern = regexp.MustCompile(`^TKT-[0-9]+$`)

type FieldError struct {
	ErrorCode    string `json:"error_code"`
	Field        string `json:"field"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
}

func (e *FieldError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func fieldError(code, field, message, suggestedFix string) *FieldError {
	return &FieldError{
		ErrorCode:    code,
		Field:        field,
		Message:      message,
		Retryable:    false,
		SuggestedFix: suggestedFix,
	}
}

type AddInput struct {
	TicketID   string
	SourcePath string
	ProofTitle string
	Note       string
	AddedAt    string
	CapturedAt string
	Actor      string
	MaxBytes   int64
}

type FileMetadata struct {
	Path      string `json:"path"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type Record struct {
	ID         string       `json:"id"`
	TicketID   string       `json:"ticket_id"`
	ProofTitle string       `json:"proof_title"`
	Note       string       `json:"note"`
	AddedAt    time.Time    `json:"added_at"`
	CapturedAt *time.Time   `json:"captured_at,omitempty"`
	Actor      string       `json:"actor,omitempty"`
	File       FileMetadata `json:"file"`
}

type Repository struct {
	RepoRoot string
}

var allowedImageMIMEs = map[string][]string{
	"image/png":  {".png"},
	"image/jpeg": {".jpg", ".jpeg"},
	"image/gif":  {".gif"},
	"image/webp": {".webp"},
}

func NewRepository(repoRoot string) *Repository {
	absRoot, _ := filepath.Abs(repoRoot)
	return &Repository{RepoRoot: absRoot}
}

func (r *Repository) Add(ctx context.Context, in AddInput) (*Record, error) {
	_ = ctx

	if !ticketIDPattern.MatchString(in.TicketID) {
		return nil, fieldError("invalid_field", "ticket_id", "ticket_id must match TKT-<number>", "use a canonical ticket ID such as TKT-240")
	}
	if strings.TrimSpace(in.SourcePath) == "" {
		return nil, fieldError("required_field", "source_path", "source_path is required", "pass --file <path> with an existing image")
	}
	if strings.TrimSpace(in.ProofTitle) == "" {
		return nil, fieldError("required_field", "proof_title", "proof_title is required", "pass a short title describing the proof")
	}
	if strings.TrimSpace(in.Note) == "" {
		return nil, fieldError("required_field", "note", "note is required", "pass one narrative note describing why this proof was attached")
	}
	if strings.TrimSpace(in.AddedAt) == "" {
		return nil, fieldError("required_field", "added_at", "added_at is required", "provide an RFC3339 timestamp")
	}

	addedAt, err := time.Parse(time.RFC3339, in.AddedAt)
	if err != nil {
		return nil, fieldError("invalid_timestamp", "added_at", "added_at must be RFC3339", "use timestamp like 2026-03-16T17:10:00Z")
	}

	var capturedAt *time.Time
	if strings.TrimSpace(in.CapturedAt) != "" {
		t, parseErr := time.Parse(time.RFC3339, in.CapturedAt)
		if parseErr != nil {
			return nil, fieldError("invalid_timestamp", "captured_at", "captured_at must be RFC3339", "use timestamp like 2026-03-16T17:09:58Z")
		}
		capturedAt = &t
	}

	sourcePath := filepath.Clean(strings.TrimSpace(in.SourcePath))
	if sourcePath == "." {
		return nil, fieldError("invalid_field", "source_path", "source_path must reference a file", "use a concrete image file path")
	}
	if filepath.IsAbs(sourcePath) {
		return nil, fieldError("unsafe_path", "source_path", "absolute source paths are not allowed", "use a repository-relative path such as fixtures/proof.png")
	}
	if sourcePath == ".." || strings.HasPrefix(sourcePath, ".."+string(filepath.Separator)) {
		return nil, fieldError("unsafe_path", "source_path", "path traversal is not allowed", "use a repository-relative path under the current repo")
	}
	absSourcePath := filepath.Join(r.RepoRoot, sourcePath)
	if !withinRoot(absSourcePath, r.RepoRoot) {
		return nil, fieldError("unsafe_path", "source_path", "source_path escapes repository root", "use a repository-relative path under the current repo")
	}

	fi, err := os.Lstat(absSourcePath)
	if err != nil {
		return nil, fieldError("missing_file", "source_path", "source_path does not exist", "verify the file path exists")
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fieldError("unsafe_path", "source_path", "symlink inputs are not allowed", "use a regular file path")
	}
	if !fi.Mode().IsRegular() {
		return nil, fieldError("invalid_file", "source_path", "source_path must be a regular file", "use a regular image file")
	}

	data, err := os.ReadFile(absSourcePath)
	if err != nil {
		return nil, fieldError("read_failed", "source_path", "failed to read source file", "check file permissions and retry")
	}
	if len(data) == 0 {
		return nil, fieldError("invalid_file", "source_path", "source file is empty", "capture and attach a non-empty image")
	}

	if in.MaxBytes > 0 && int64(len(data)) > in.MaxBytes {
		return nil, fieldError("proof_too_large", "size_bytes", fmt.Sprintf("proof file exceeds max size of %d bytes", in.MaxBytes), "compress the image or raise the max size setting")
	}

	mimeType := sniffMIME(data)
	allowedExts, ok := allowedImageMIMEs[mimeType]
	if !ok {
		return nil, fieldError("unsupported_media_type", "mime_type", fmt.Sprintf("unsupported media type %q", mimeType), "attach a PNG, JPEG, GIF, WEBP, or other image media")
	}
	sourceExt := sanitizeExt(filepath.Ext(sourcePath))
	if sourceExt != "" && !containsString(allowedExts, sourceExt) {
		return nil, fieldError("mime_extension_mismatch", "mime_type", fmt.Sprintf("file extension %q does not match detected media type %q", sourceExt, mimeType), "use a file extension that matches the actual image bytes")
	}

	hash := sha256.Sum256(data)
	sha := hex.EncodeToString(hash[:])

	records, err := r.load(in.TicketID)
	if err != nil {
		return nil, err
	}
	id := r.nextProofID(records, addedAt, sha)

	ext := allowedExts[0]
	if ext == "" {
		ext = sanitizeExt(filepath.Ext(sourcePath))
	}
	if ext == "" {
		ext = ".img"
	}

	relPath := filepath.ToSlash(filepath.Join(".docket", "proofs", "by-hash", sha+ext))
	absPath := filepath.Join(r.RepoRoot, filepath.FromSlash(relPath))
	if !withinRoot(absPath, r.RepoRoot) {
		return nil, fieldError("unsafe_path", "path", "resolved proof path escapes repository root", "use a valid repository path")
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fieldError("io_failed", "path", "failed to create proof storage directory", "check repository write permissions")
	}
	if _, statErr := os.Stat(absPath); statErr != nil {
		if !os.IsNotExist(statErr) {
			return nil, fieldError("io_failed", "path", "failed to stat proof blob path", "check repository permissions and retry")
		}
		if err := os.WriteFile(absPath, data, 0o644); err != nil {
			return nil, fieldError("io_failed", "path", "failed to write proof file", "check repository write permissions")
		}
	}

	rec := Record{
		ID:         id,
		TicketID:   in.TicketID,
		ProofTitle: strings.TrimSpace(in.ProofTitle),
		Note:       strings.TrimSpace(in.Note),
		AddedAt:    addedAt.UTC().Truncate(time.Second),
		CapturedAt: capturedAt,
		Actor:      strings.TrimSpace(in.Actor),
		File: FileMetadata{
			Path:      relPath,
			MIMEType:  mimeType,
			SizeBytes: int64(len(data)),
			SHA256:    sha,
		},
	}

	records = append(records, rec)
	sortRecords(records)
	if err := r.save(in.TicketID, records); err != nil {
		return nil, err
	}

	return &rec, nil
}

func (r *Repository) List(ctx context.Context, ticketID string) ([]Record, error) {
	_ = ctx
	if !ticketIDPattern.MatchString(ticketID) {
		return nil, fieldError("invalid_field", "ticket_id", "ticket_id must match TKT-<number>", "use a canonical ticket ID such as TKT-240")
	}
	records, err := r.load(ticketID)
	if err != nil {
		return nil, err
	}
	sortRecords(records)
	out := make([]Record, len(records))
	copy(out, records)
	return out, nil
}

func (r *Repository) metadataPath(ticketID string) string {
	return filepath.Join(r.RepoRoot, ".docket", "proofs", ticketID, "metadata.json")
}

func (r *Repository) load(ticketID string) ([]Record, error) {
	path := r.metadataPath(ticketID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, fieldError("io_failed", "metadata", "failed to read proof metadata", "verify repository permissions and retry")
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fieldError("malformed_metadata", "metadata", "proof metadata is malformed", "repair metadata JSON and retry")
	}
	return records, nil
}

func (r *Repository) save(ticketID string, records []Record) error {
	path := r.metadataPath(ticketID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fieldError("io_failed", "metadata", "failed to create proof metadata directory", "check repository write permissions")
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fieldError("io_failed", "metadata", "failed to serialize proof metadata", "retry operation")
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fieldError("io_failed", "metadata", "failed to write proof metadata", "check repository write permissions")
	}
	return nil
}

func (r *Repository) nextProofID(records []Record, addedAt time.Time, sha string) string {
	base := fmt.Sprintf("PRF-%s-%s", addedAt.UTC().Format("20060102T150405Z"), sha[:12])
	candidate := base
	index := 2
	existing := make(map[string]struct{}, len(records))
	for _, rec := range records {
		existing[rec.ID] = struct{}{}
	}
	for {
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].AddedAt.Equal(records[j].AddedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].AddedAt.Before(records[j].AddedAt)
	})
}

func sniffMIME(data []byte) string {
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return http.DetectContentType(sample)
}

func sanitizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" || ext == "." {
		return ""
	}
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		return ""
	}
	return ext
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func withinRoot(path, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
