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
//
// This is the default filter used by the interceptors. Use WithErrorFilter
// to provide a custom filter.
func ShouldCaptureError(err error) bool {
	switch status.Code(err) {
	case codes.Internal, codes.Unknown, codes.Unavailable, codes.DataLoss, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

// InterceptorOption configures the Sentry gRPC interceptors.
type InterceptorOption func(*interceptorConfig)

type interceptorConfig struct {
	errorFilter func(error) bool
}

func defaultConfig() interceptorConfig {
	return interceptorConfig{
		errorFilter: ShouldCaptureError,
	}
}

// WithErrorFilter sets a custom function that decides which errors are
// reported to Sentry. It replaces the default ShouldCaptureError filter.
func WithErrorFilter(f func(error) bool) InterceptorOption {
	return func(c *interceptorConfig) {
		c.errorFilter = f
	}
}

// spanStatus maps a gRPC error to the appropriate Sentry span status.
// Returns SpanStatusOK when err is nil.
func spanStatus(err error) sentry.SpanStatus {
	if err == nil {
		return sentry.SpanStatusOK
	}
	switch status.Code(err) {
	case codes.InvalidArgument:
		return sentry.SpanStatusInvalidArgument
	case codes.DeadlineExceeded:
		return sentry.SpanStatusDeadlineExceeded
	case codes.NotFound:
		return sentry.SpanStatusNotFound
	case codes.AlreadyExists:
		return sentry.SpanStatusAlreadyExists
	case codes.PermissionDenied:
		return sentry.SpanStatusPermissionDenied
	case codes.ResourceExhausted:
		return sentry.SpanStatusResourceExhausted
	case codes.Aborted:
		return sentry.SpanStatusAborted
	case codes.Unimplemented:
		return sentry.SpanStatusUnimplemented
	case codes.Unavailable:
		return sentry.SpanStatusUnavailable
	case codes.Unauthenticated:
		return sentry.SpanStatusUnauthenticated
	case codes.Canceled:
		return sentry.SpanStatusCanceled
	default:
		return sentry.SpanStatusInternalError
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
// continues a Sentry distributed trace from incoming metadata, populates
// outgoing metadata with trace headers, captures panics, and reports
// server-side errors to Sentry.
//
// Note: panic stack traces in Sentry will point to the recovery site
// (the interceptor), not the original panic location. This is a known
// Go runtime limitation when using explicit recover().
func UnaryServerInterceptor(opts ...InterceptorOption) grpc.UnaryServerInterceptor {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
		}
		ctx = sentry.SetHubOnContext(ctx, hub)

		sentryTrace, baggage := extractTraceHeaders(ctx)

		span := sentry.StartTransaction(ctx, info.FullMethod,
			sentry.ContinueFromHeaders(sentryTrace, baggage),
			sentry.WithOpName("grpc.server"),
		)
		defer func() {
			if r := recover(); r != nil {
				hub.RecoverWithContext(ctx, r)
				span.Status = sentry.SpanStatusInternalError
				span.Finish()
				err = status.Errorf(codes.Internal, "panic: %v", r)
				return
			}
			span.Status = spanStatus(err)
			span.Finish()
		}()

		ctx = span.Context()
		ctx = sentry.SetHubOnContext(ctx, hub)
		ctx = metadata.AppendToOutgoingContext(ctx,
			"sentry-trace", span.ToSentryTrace(),
			"baggage", span.ToBaggage(),
		)

		resp, err = handler(ctx, req)
		if err != nil && cfg.errorFilter(err) {
			hub.CaptureException(err)
		}
		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that
// continues a Sentry distributed trace from incoming metadata, populates
// outgoing metadata with trace headers, captures panics, and reports
// server-side errors to Sentry.
func StreamServerInterceptor(opts ...InterceptorOption) grpc.StreamServerInterceptor {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ctx := ss.Context()

		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
		}
		ctx = sentry.SetHubOnContext(ctx, hub)

		sentryTrace, baggage := extractTraceHeaders(ctx)

		span := sentry.StartTransaction(ctx, info.FullMethod,
			sentry.ContinueFromHeaders(sentryTrace, baggage),
			sentry.WithOpName("grpc.server"),
		)
		defer func() {
			if r := recover(); r != nil {
				hub.RecoverWithContext(ctx, r)
				span.Status = sentry.SpanStatusInternalError
				span.Finish()
				err = status.Errorf(codes.Internal, "panic: %v", r)
				return
			}
			span.Status = spanStatus(err)
			span.Finish()
		}()

		ctx = span.Context()
		ctx = sentry.SetHubOnContext(ctx, hub)
		ctx = metadata.AppendToOutgoingContext(ctx,
			"sentry-trace", span.ToSentryTrace(),
			"baggage", span.ToBaggage(),
		)

		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}

		err = handler(srv, wrapped)
		if err != nil && cfg.errorFilter(err) {
			hub.CaptureException(err)
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
