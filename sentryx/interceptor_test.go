package sentryx

import (
	"context"
	"fmt"
	"testing"

	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func init() {
	// Initialize Sentry with a noop transport so interceptors can create spans.
	sentry.Init(sentry.ClientOptions{
		Dsn:              "https://key@sentry.io/1",
		EnableTracing:    true,
		TracesSampleRate: 1.0,
	})
}

func TestShouldCaptureError(t *testing.T) {
	tests := []struct {
		code codes.Code
		want bool
	}{
		{codes.Internal, true},
		{codes.Unknown, true},
		{codes.Unavailable, true},
		{codes.DataLoss, true},
		{codes.ResourceExhausted, true},
		{codes.InvalidArgument, false},
		{codes.NotFound, false},
		{codes.OK, false},
		{codes.Canceled, false},
		{codes.PermissionDenied, false},
		{codes.Unauthenticated, false},
		{codes.AlreadyExists, false},
		{codes.FailedPrecondition, false},
		{codes.Aborted, false},
		{codes.OutOfRange, false},
		{codes.Unimplemented, false},
		{codes.DeadlineExceeded, false},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			err := status.Error(tt.code, "test")
			if got := ShouldCaptureError(err); got != tt.want {
				t.Fatalf("ShouldCaptureError(%v) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestExtractTraceHeaders(t *testing.T) {
	t.Run("no metadata", func(t *testing.T) {
		st, bg := extractTraceHeaders(context.Background())
		if st != "" || bg != "" {
			t.Fatalf("expected empty strings, got %q, %q", st, bg)
		}
	})

	t.Run("with trace headers", func(t *testing.T) {
		md := metadata.Pairs(
			"sentry-trace", "abc123",
			"baggage", "sentry-environment=prod",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)
		st, bg := extractTraceHeaders(ctx)
		if st != "abc123" {
			t.Fatalf("sentry-trace = %q, want %q", st, "abc123")
		}
		if bg != "sentry-environment=prod" {
			t.Fatalf("baggage = %q, want %q", bg, "sentry-environment=prod")
		}
	})

	t.Run("partial headers", func(t *testing.T) {
		md := metadata.Pairs("sentry-trace", "abc123")
		ctx := metadata.NewIncomingContext(context.Background(), md)
		st, bg := extractTraceHeaders(ctx)
		if st != "abc123" {
			t.Fatalf("sentry-trace = %q, want %q", st, "abc123")
		}
		if bg != "" {
			t.Fatalf("baggage = %q, want empty", bg)
		}
	})
}

func TestUnaryServerInterceptor_Success(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"sentry-trace", "abc123",
		"baggage", "sentry-environment=test",
	))

	resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		return "response", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Fatalf("got %v, want %q", resp, "response")
	}
}

func TestUnaryServerInterceptor_Error(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.Internal, "boom")
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", status.Code(err))
	}
}

func TestUnaryServerInterceptor_ClientError(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.InvalidArgument, "bad request")
	})

	if err == nil {
		t.Fatal("expected error")
	}
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

func TestStreamServerInterceptor_Success(t *testing.T) {
	interceptor := StreamServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"sentry-trace", "abc123",
		"baggage", "sentry-environment=test",
	))

	err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}, func(srv any, stream grpc.ServerStream) error {
		// Verify the stream has an enriched context.
		if stream.Context() == ctx {
			t.Error("expected wrapped context, got original")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamServerInterceptor_Error(t *testing.T) {
	interceptor := StreamServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}, func(srv any, stream grpc.ServerStream) error {
		return status.Error(codes.Internal, "stream error")
	})

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWrappedServerStream_Context(t *testing.T) {
	ctx := context.WithValue(context.Background(), "key", "value")
	ss := &mockServerStream{ctx: context.Background()}
	wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

	if wrapped.Context() != ctx {
		t.Fatal("wrapped stream should return injected context")
	}
}

func TestUnaryServerInterceptor_NoMetadata(t *testing.T) {
	interceptor := UnaryServerInterceptor()

	resp, err := interceptor(context.Background(), "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("got %v, want %q", resp, "ok")
	}
}

func TestStreamServerInterceptor_NoMetadata(t *testing.T) {
	interceptor := StreamServerInterceptor()

	err := interceptor(nil, &mockServerStream{ctx: context.Background()}, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}, func(srv any, stream grpc.ServerStream) error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShouldCaptureError_NilError(t *testing.T) {
	if ShouldCaptureError(nil) {
		t.Fatal("nil error should not be captured")
	}
}

func TestShouldCaptureError_NonStatusError(t *testing.T) {
	// A plain error gets codes.Unknown from status.Code.
	if !ShouldCaptureError(fmt.Errorf("plain error")) {
		t.Fatal("plain error (Unknown code) should be captured")
	}
}
