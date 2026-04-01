package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func maybeAutoTransitionReviewReady(
	ctx context.Context,
	out io.Writer,
	s *local.Store,
	cfg *ticket.Config,
	t *ticket.Ticket,
	actor string,
) (*ticket.Ticket, bool) {
	_ = ctx
	_ = out
	_ = s
	_ = cfg
	_ = actor
	return t, false
}

func closeoutReadinessFailures(t *ticket.Ticket, cfg *ticket.Config) []string {
	failures := []string{}
	if !t.ACComplete() {
		failures = append(failures, "acceptance criteria incomplete")
	}
	handoff := strings.TrimSpace(t.Handoff)
	if handoff == "" {
		failures = append(failures, "handoff missing")
		return failures
	}
	lower := strings.ToLower(handoff)
	for _, section := range cfg.HandoffSections {
		if strings.Contains(lower, strings.ToLower(section)) {
			continue
		}
		failures = append(failures, fmt.Sprintf("handoff missing section %s", section))
	}
	return failures
}
