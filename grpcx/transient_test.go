package grpcx

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unavailable", status.Error(codes.Unavailable, "unavailable"), true},
		{"aborted", status.Error(codes.Aborted, "aborted"), true},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "exhausted"), true},
		{"not found", status.Error(codes.NotFound, "not found"), false},
		{"internal", status.Error(codes.Internal, "internal"), false},
		{"permission denied", status.Error(codes.PermissionDenied, "denied"), false},
		{"ok", status.Error(codes.OK, "ok"), false},
		{"non-grpc error", fmt.Errorf("plain error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransient(tt.err)
			if got != tt.want {
				t.Errorf("IsTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
