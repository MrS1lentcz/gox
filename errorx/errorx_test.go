package errorx

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestNew_Stderr(t *testing.T) {
	r, err := New("stderr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.(*stderrReporter); !ok {
		t.Fatalf("expected *stderrReporter, got %T", r)
	}
}

func TestNew_Sentry(t *testing.T) {
	r, err := New("sentry:https://key@sentry.io/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.(*sentryReporter); !ok {
		t.Fatalf("expected *sentryReporter, got %T", r)
	}
}

func TestNew_SentryEmptyDSN(t *testing.T) {
	_, err := New("sentry:")
	if err == nil {
		t.Fatal("expected error for empty sentry DSN")
	}
	if !strings.Contains(err.Error(), "empty sentry DSN") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNew_SentryInvalidDSN(t *testing.T) {
	_, err := New("sentry:not-a-valid-dsn")
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestNew_UnknownFormat(t *testing.T) {
	_, err := New("file:/tmp/errors.log")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown config format") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNew_EmptyString(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestStderrReporter_Report(t *testing.T) {
	var buf bytes.Buffer
	r := &stderrReporter{
		logger: log.New(&buf, "", 0),
	}

	testErr := errors.New("something went wrong")
	r.Report(testErr)

	got := strings.TrimSpace(buf.String())
	if got != "something went wrong" {
		t.Fatalf("got %q, want %q", got, "something went wrong")
	}
}

func TestStderrReporter_Close(t *testing.T) {
	r := &stderrReporter{
		logger: log.New(&bytes.Buffer{}, "", 0),
	}
	r.Close() // should not panic
}

func TestSentryReporter_Report(t *testing.T) {
	r, err := New("sentry:https://key@sentry.io/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r.Report(errors.New("test error")) // should not panic
}

func TestSentryReporter_Close(t *testing.T) {
	r, err := New("sentry:https://key@sentry.io/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r.Close() // should not panic
}
