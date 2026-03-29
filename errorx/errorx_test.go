package errorx

import (
	"bytes"
	"errors"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// resetGlobal resets global reporter state between tests.
func resetGlobal() {
	globalReporter = nil
	reporterOnce = sync.Once{}
}

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

func TestSetReporter(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	var buf bytes.Buffer
	r := &stderrReporter{logger: log.New(&buf, "", 0)}
	SetReporter(r)

	ReportError(errors.New("test"))
	if !strings.Contains(buf.String(), "test") {
		t.Fatalf("expected error in buffer, got %q", buf.String())
	}
}

func TestSetReporter_OnlyFirstCallTakesEffect(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	var buf1, buf2 bytes.Buffer
	r1 := &stderrReporter{logger: log.New(&buf1, "", 0)}
	r2 := &stderrReporter{logger: log.New(&buf2, "", 0)}

	SetReporter(r1)
	SetReporter(r2)

	ReportError(errors.New("hello"))
	if !strings.Contains(buf1.String(), "hello") {
		t.Fatal("expected first reporter to be used")
	}
	if buf2.Len() != 0 {
		t.Fatal("second reporter should not have been used")
	}
}

func TestReportError_FallbackToStderr(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	// ReportError without SetReporter should not panic
	ReportError(errors.New("fallback error"))
}

func TestCloseReporter(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	// CloseReporter without SetReporter should not panic
	CloseReporter()

	var buf bytes.Buffer
	r := &stderrReporter{logger: log.New(&buf, "", 0)}
	SetReporter(r)
	CloseReporter() // should not panic
}

func TestMustInit_Stderr(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	MustInit("stderr")

	if globalReporter == nil {
		t.Fatal("expected globalReporter to be set")
	}
	if _, ok := globalReporter.(*stderrReporter); !ok {
		t.Fatalf("expected *stderrReporter, got %T", globalReporter)
	}
}

func TestMustInit_InvalidConfig(t *testing.T) {
	if os.Getenv("TEST_MUST_INIT_FATAL") == "1" {
		MustInit("invalid")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMustInit_InvalidConfig$")
	cmd.Env = append(os.Environ(), "TEST_MUST_INIT_FATAL=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && !exitErr.Success() {
		return
	}
	t.Fatal("expected process to exit with non-zero status")
}
