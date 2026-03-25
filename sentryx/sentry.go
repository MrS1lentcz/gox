package sentryx

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
)

// DefaultClientOptions returns a sentry.ClientOptions with sensible defaults:
// EnableTracing is set to true and TracesSampleRate to 0.2.
// The returned value can be further customized before passing to Init.
func DefaultClientOptions(dsn, serverName string) sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn:              dsn,
		ServerName:       serverName,
		EnableTracing:    true,
		TracesSampleRate: 0.2,
	}
}

// Init initializes the Sentry SDK with the given options.
// If opts.Dsn is empty, initialization is skipped and no error is returned.
func Init(opts sentry.ClientOptions) error {
	if opts.Dsn == "" {
		return nil
	}
	if err := sentry.Init(opts); err != nil {
		return fmt.Errorf("sentryx: init failed: %w", err)
	}
	return nil
}

// Flush waits for buffered events to be sent to Sentry.
// It blocks for at most the given duration (or 2 seconds if zero).
// It returns true if all events were flushed within the timeout.
func Flush(timeout time.Duration) bool {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return sentry.Flush(timeout)
}
