package ticket

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var canonicalTicketIDPattern = regexp.MustCompile(`(?i)^TKT-([0-9]+)$`)

// FormatID returns "TKT-NNN" for a given sequence number.
// Pads to 3 digits; beyond 999 produces TKT-1000 etc. — acceptable.
func FormatID(seq int) string {
	return fmt.Sprintf("TKT-%03d", seq)
}

// NormalizeID accepts common ticket identifier variants and returns canonical TKT-NNN form.
// Accepted examples: "TKT-1", "tkt-001", "[TKT-42]", "42".
func NormalizeID(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && len(s) > 2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if m := canonicalTicketIDPattern.FindStringSubmatch(s); len(m) == 2 {
		seq, err := strconv.Atoi(m[1])
		if err != nil || seq <= 0 {
			return "", false
		}
		return FormatID(seq), true
	}
	if seq, err := strconv.Atoi(s); err == nil && seq > 0 {
		return FormatID(seq), true
	}
	return "", false
}

// NextID increments the counter in config.json and returns the new ticket ID + seq.
// Not atomic across concurrent processes — acceptable for solo/small-team use.
func NextID(repoRoot string) (id string, seq int, err error) {
	cfg, err := LoadConfig(repoRoot)
	if err != nil {
		return "", 0, err
	}
	cfg.Counter++
	if err := SaveConfig(repoRoot, cfg); err != nil {
		return "", 0, fmt.Errorf("failed to increment counter: %w", err)
	}
	return FormatID(cfg.Counter), cfg.Counter, nil
}
