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

func TestShouldCaptureError_NilError(t *testing.T) {
	if ShouldCaptureError(nil) {
		t.Fatal("nil error should not be captured")
	}
}

func TestShouldCaptureError_NonStatusError(t *testing.T) {
	if !ShouldCaptureError(fmt.Errorf("plain error")) {
		t.Fatal("plain error (Unknown code) should be captured")
	}
}

func TestSpanStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want sentry.SpanStatus
	}{
		{"nil", nil, sentry.SpanStatusOK},
		{"InvalidArgument", status.Error(codes.InvalidArgument, ""), sentry.SpanStatusInvalidArgument},
		{"DeadlineExceeded", status.Error(codes.DeadlineExceeded, ""), sentry.SpanStatusDeadlineExceeded},
		{"NotFound", status.Error(codes.NotFound, ""), sentry.SpanStatusNotFound},
		{"AlreadyExists", status.Error(codes.AlreadyExists, ""), sentry.SpanStatusAlreadyExists},
		{"PermissionDenied", status.Error(codes.PermissionDenied, ""), sentry.SpanStatusPermissionDenied},
		{"ResourceExhausted", status.Error(codes.ResourceExhausted, ""), sentry.SpanStatusResourceExhausted},
		{"Aborted", status.Error(codes.Aborted, ""), sentry.SpanStatusAborted},
		{"Unimplemented", status.Error(codes.Unimplemented, ""), sentry.SpanStatusUnimplemented},
		{"Unavailable", status.Error(codes.Unavailable, ""), sentry.SpanStatusUnavailable},
		{"Unauthenticated", status.Error(codes.Unauthenticated, ""), sentry.SpanStatusUnauthenticated},
		{"Canceled", status.Error(codes.Canceled, ""), sentry.SpanStatusCanceled},
		{"Internal", status.Error(codes.Internal, ""), sentry.SpanStatusInternalError},
		{"Unknown", status.Error(codes.Unknown, ""), sentry.SpanStatusInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spanStatus(tt.err); got != tt.want {
				t.Fatalf("spanStatus() = %v, want %v", got, tt.want)
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

func TestWithErrorFilter(t *testing.T) {
	// Custom filter that only captures NotFound.
	filter := WithErrorFilter(func(err error) bool {
		return status.Code(err) == codes.NotFound
	})

	interceptor := UnaryServerInterceptor(filter)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	// NotFound should be captured (no crash, just verifying the filter is called).
	_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWithErrorFilter_Stream(t *testing.T) {
	filter := WithErrorFilter(func(err error) bool {
		return status.Code(err) == codes.NotFound
	})

	interceptor := StreamServerInterceptor(filter)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}, func(srv any, stream grpc.ServerStream) error {
		return status.Error(codes.NotFound, "not found")
	})
	if err == nil {
		t.Fatal("expected error")
	}
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

func TestUnaryServerInterceptor_Panic(t *testing.T) {
	interceptor := UnaryServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	_, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}, func(ctx context.Context, req any) (any, error) {
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected error after panic")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", status.Code(err))
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

func TestStreamServerInterceptor_Panic(t *testing.T) {
	interceptor := StreamServerInterceptor()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(nil))

	err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/StreamMethod",
	}, func(srv any, stream grpc.ServerStream) error {
		panic("stream panic")
	})

	if err == nil {
		t.Fatal("expected error after panic")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", status.Code(err))
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

type ctxKey struct{}

func TestWrappedServerStream_Context(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	ss := &mockServerStream{ctx: context.Background()}
	wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

	if wrapped.Context() != ctx {
		t.Fatal("wrapped stream should return injected context")
	}
}
