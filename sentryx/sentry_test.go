package sentryx

import (
	"testing"
	"time"
)

func TestInit_EmptyDSN(t *testing.T) {
	err := Init("", "test-server")
	if err != nil {
		t.Fatalf("expected no error for empty DSN, got %v", err)
	}
}

func TestInit_InvalidDSN(t *testing.T) {
	err := Init("not-a-valid-dsn", "test-server")
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestInit_ValidDSN(t *testing.T) {
	// Use a syntactically valid DSN (won't actually connect).
	err := Init("https://key@sentry.io/1", "test-server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlush_DefaultTimeout(t *testing.T) {
	// Just ensure it doesn't panic; Sentry client may or may not be initialized.
	Flush(0)
}

func TestFlush_CustomTimeout(t *testing.T) {
	Flush(100 * time.Millisecond)
}
