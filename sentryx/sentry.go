package sentryx

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
)

// Init initializes the Sentry SDK with the given DSN and server name.
// If dsn is empty, initialization is skipped and no error is returned.
func Init(dsn, serverName string) error {
	if dsn == "" {
		return nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		ServerName:       serverName,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
	})
	if err != nil {
		return fmt.Errorf("sentryx: init failed: %w", err)
	}
	return nil
}

// Flush waits for buffered events to be sent to Sentry.
// It blocks for at most the given duration (or 2 seconds if zero).
func Flush(timeout time.Duration) {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	sentry.Flush(timeout)
}
