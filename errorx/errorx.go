package errorx

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mrs1lentcz/gox/sentryx"

	"github.com/getsentry/sentry-go"
)

// Reporter reports errors to a configured backend.
type Reporter interface {
	// Report sends the error to the configured backend.
	Report(err error)
	// Close flushes any buffered data and releases resources.
	Close()
}

// New parses the config string and returns the appropriate Reporter.
//
// Supported formats:
//   - "stderr"            — logs errors to stderr
//   - "sentry:<dsn>"      — sends errors to Sentry using the given DSN
func New(config string) (Reporter, error) {
	switch {
	case config == "stderr":
		return newStderrReporter(), nil
	case strings.HasPrefix(config, "sentry:"):
		dsn := strings.TrimPrefix(config, "sentry:")
		if dsn == "" {
			return nil, fmt.Errorf("errorx: empty sentry DSN")
		}
		return newSentryReporter(dsn)
	default:
		return nil, fmt.Errorf("errorx: unknown config format: %q", config)
	}
}

type stderrReporter struct {
	logger *log.Logger
}

func newStderrReporter() *stderrReporter {
	return &stderrReporter{
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

func (r *stderrReporter) Report(err error) {
	r.logger.Println(err)
}

func (r *stderrReporter) Close() {}

type sentryReporter struct{}

func newSentryReporter(dsn string) (*sentryReporter, error) {
	opts := sentryx.DefaultClientOptions(dsn, "")
	if err := sentryx.Init(opts); err != nil {
		return nil, fmt.Errorf("errorx: %w", err)
	}
	return &sentryReporter{}, nil
}

func (r *sentryReporter) Report(err error) {
	sentry.CaptureException(err)
}

func (r *sentryReporter) Close() {
	sentryx.Flush(2 * time.Second)
}
