package sentryx

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestInit_EmptyDSN(t *testing.T) {
	err := Init(sentry.ClientOptions{})
	if err != nil {
		t.Fatalf("expected no error for empty DSN, got %v", err)
	}
}

func TestInit_InvalidDSN(t *testing.T) {
	err := Init(sentry.ClientOptions{Dsn: "not-a-valid-dsn"})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestInit_ValidDSN(t *testing.T) {
	err := Init(sentry.ClientOptions{
		Dsn:              "https://key@sentry.io/1",
		EnableTracing:    true,
		TracesSampleRate: 0.5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultClientOptions(t *testing.T) {
	opts := DefaultClientOptions("https://key@sentry.io/1", "test-server")
	if opts.Dsn != "https://key@sentry.io/1" {
		t.Fatalf("Dsn = %q, want %q", opts.Dsn, "https://key@sentry.io/1")
	}
	if opts.ServerName != "test-server" {
		t.Fatalf("ServerName = %q, want %q", opts.ServerName, "test-server")
	}
	if !opts.EnableTracing {
		t.Fatal("EnableTracing should be true")
	}
	if opts.TracesSampleRate != 0.2 {
		t.Fatalf("TracesSampleRate = %v, want 0.2", opts.TracesSampleRate)
	}
}

func TestDefaultClientOptions_Customizable(t *testing.T) {
	opts := DefaultClientOptions("https://key@sentry.io/1", "test-server")
	opts.TracesSampleRate = 0.5
	opts.Environment = "staging"

	err := Init(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlush_DefaultTimeout(t *testing.T) {
	_ = Flush(0)
}

func TestFlush_CustomTimeout(t *testing.T) {
	_ = Flush(100 * time.Millisecond)
}
