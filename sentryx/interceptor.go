package sentryx

import (
	"context"

	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ShouldCaptureError reports whether a gRPC error represents a server-side
// failure that should be reported to Sentry. Client errors (InvalidArgument,
// NotFound, etc.) and cancellations are considered expected.
func ShouldCaptureError(err error) bool {
	switch status.Code(err) {
	case codes.Internal, codes.Unknown, codes.Unavailable, codes.DataLoss, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

func extractTraceHeaders(ctx context.Context) (sentryTrace, baggage string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}
	if vals := md.Get("sentry-trace"); len(vals) > 0 {
		sentryTrace = vals[0]
	}
	if vals := md.Get("baggage"); len(vals) > 0 {
		baggage = vals[0]
	}
	return sentryTrace, baggage
}

// UnaryServerInterceptor returns a gRPC unary server interceptor that
// continues a Sentry distributed trace from incoming metadata and forwards
// trace headers to outgoing gRPC calls made by the handler.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		sentryTrace, baggage := extractTraceHeaders(ctx)

		span := sentry.StartTransaction(ctx, info.FullMethod,
			sentry.ContinueFromHeaders(sentryTrace, baggage),
			sentry.WithOpName("grpc.server"),
		)
		defer span.Finish()

		ctx = span.Context()
		ctx = metadata.AppendToOutgoingContext(ctx,
			"sentry-trace", span.ToSentryTrace(),
			"baggage", span.ToBaggage(),
		)

		resp, err := handler(ctx, req)
		if err != nil {
			span.Status = sentry.SpanStatusInternalError
			if ShouldCaptureError(err) {
				sentry.CaptureException(err)
			}
		}
		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that
// continues a Sentry distributed trace from incoming metadata.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		sentryTrace, baggage := extractTraceHeaders(ctx)

		span := sentry.StartTransaction(ctx, info.FullMethod,
			sentry.ContinueFromHeaders(sentryTrace, baggage),
			sentry.WithOpName("grpc.server"),
		)
		defer span.Finish()

		ctx = span.Context()
		ctx = metadata.AppendToOutgoingContext(ctx,
			"sentry-trace", span.ToSentryTrace(),
			"baggage", span.ToBaggage(),
		)

		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

		err := handler(srv, wrapped)
		if err != nil {
			span.Status = sentry.SpanStatusInternalError
			if ShouldCaptureError(err) {
				sentry.CaptureException(err)
			}
		}
		return err
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
