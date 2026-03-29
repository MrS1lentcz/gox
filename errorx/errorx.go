package errorx

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mrs1lentcz/gox/sentryx"

	"github.com/getsentry/sentry-go"
)

var (
	globalReporter Reporter
	reporterOnce   sync.Once
)

// SetReporter sets the global error reporter. Only the first call takes effect.
func SetReporter(r Reporter) {
	reporterOnce.Do(func() {
		globalReporter = r
	})
}

// MustInit parses the config string, creates a Reporter, and sets it as the
// global reporter. It calls log.Fatalf on any error — intended for use during
// application bootstrap.
func MustInit(config string) {
	r, err := New(config)
	if err != nil {
		log.Fatalf("errorx: %v", err)
	}
	SetReporter(r)
}

// ReportError reports an error using the global reporter.
// Falls back to stderr if SetReporter was never called.
func ReportError(err error) {
	if globalReporter != nil {
		globalReporter.Report(err)
		return
	}
	log.Printf("error: %v", err)
}

// CloseReporter flushes and closes the global reporter.
func CloseReporter() {
	if globalReporter != nil {
		globalReporter.Close()
	}
}

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
