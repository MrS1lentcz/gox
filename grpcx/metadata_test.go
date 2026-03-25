package grpcx

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGetMetadataValue(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		key     string
		want    string
		wantErr codes.Code
	}{
		{
			name:    "no metadata in context",
			ctx:     context.Background(),
			key:     "token",
			wantErr: codes.InvalidArgument,
		},
		{
			name:    "key missing from metadata",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "val")),
			key:     "token",
			wantErr: codes.InvalidArgument,
		},
		{
			name: "key present",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs("token", "abc123")),
			key:  "token",
			want: "abc123",
		},
		{
			name: "multiple values returns first",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs("token", "first", "token", "second")),
			key:  "token",
			want: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMetadataValue(tt.ctx, tt.key)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error with code %v, got nil", tt.wantErr)
				}
				if c := status.Code(err); c != tt.wantErr {
					t.Fatalf("expected code %v, got %v", tt.wantErr, c)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasMetadataKey(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		key  string
		want bool
	}{
		{
			name: "no metadata",
			ctx:  context.Background(),
			key:  "token",
			want: false,
		},
		{
			name: "key missing",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "val")),
			key:  "token",
			want: false,
		},
		{
			name: "key present",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs("token", "abc")),
			key:  "token",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasMetadataKey(tt.ctx, tt.key); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
