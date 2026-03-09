package ticket

import "fmt"

// FormatID returns "TKT-NNN" for a given sequence number.
// Pads to 3 digits; beyond 999 produces TKT-1000 etc. — acceptable.
func FormatID(seq int) string {
	return fmt.Sprintf("TKT-%03d", seq)
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
