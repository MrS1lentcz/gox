package grpcx

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsTransient reports whether the given error is a gRPC error with a status
// code that indicates a transient failure worth retrying:
//
//   - codes.Unavailable
//   - codes.Aborted
//   - codes.ResourceExhausted
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}
