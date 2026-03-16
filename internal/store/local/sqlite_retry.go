package local

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	defaultSQLiteBusyRetries = 4
	defaultSQLiteBusyBackoff = 20 * time.Millisecond
	maxSQLiteBusyBackoff     = 250 * time.Millisecond
)

var (
	sqliteBusyMaxRetries = defaultSQLiteBusyRetries
	sqliteBusyBaseDelay  = defaultSQLiteBusyBackoff
	sqliteBusySleep      = time.Sleep
)

func withSQLiteBusyRetry(ctx context.Context, operation string, fn func() error) error {
	attempts := sqliteBusyMaxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableSQLiteBusy(err) || attempt == attempts {
			break
		}

		delay := sqliteBusyBaseDelay * time.Duration(1<<(attempt-1))
		if delay > maxSQLiteBusyBackoff {
			delay = maxSQLiteBusyBackoff
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s aborted after %d attempt(s): %w", operation, attempt, ctx.Err())
		default:
		}
		sqliteBusySleep(delay)
	}

	if lastErr == nil {
		return nil
	}
	if isRetryableSQLiteBusy(lastErr) {
		return fmt.Errorf("%s failed after %d attempts due to SQLITE_BUSY: %w", operation, attempts, lastErr)
	}
	return lastErr
}

func isRetryableSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked")
}
