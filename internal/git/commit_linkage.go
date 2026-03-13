package git

import (
	"fmt"
	"strings"
)

// HasTicketTrailerSince returns true when at least one commit on ref since the given
// timestamp includes `Ticket: <ticketID>` in commit trailers.
func HasTicketTrailerSince(repoRoot, ref, ticketID, sinceRFC3339 string) (bool, error) {
	if strings.TrimSpace(ticketID) == "" {
		return false, fmt.Errorf("ticket ID is required")
	}
	if strings.TrimSpace(ref) == "" {
		return false, fmt.Errorf("ref is required")
	}

	args := []string{"log", "--format=%(trailers:key=Ticket,valueonly)", ref}
	if strings.TrimSpace(sinceRFC3339) != "" {
		args = append(args, "--since="+sinceRFC3339)
	}
	out, err := runGit(repoRoot, args...)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) == ticketID {
				return true, nil
			}
		}
		return false, nil
	}

	// Fallback for older git versions lacking trailer format support.
	shaArgs := []string{"log", "--format=%H", ref}
	if strings.TrimSpace(sinceRFC3339) != "" {
		shaArgs = append(shaArgs, "--since="+sinceRFC3339)
	}
	shaOut, shaErr := runGit(repoRoot, shaArgs...)
	if shaErr != nil {
		return false, err
	}
	for _, line := range strings.Split(shaOut, "\n") {
		sha := strings.TrimSpace(line)
		if sha == "" {
			continue
		}
		tid, trailerErr := CommitTicket(repoRoot, sha)
		if trailerErr != nil {
			return false, trailerErr
		}
		if tid == ticketID {
			return true, nil
		}
	}
	return false, nil
}
