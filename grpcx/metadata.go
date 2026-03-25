package grpcx

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GetMetadataValue extracts a single value for the given key from incoming
// gRPC metadata. It returns a gRPC InvalidArgument error when the metadata
// is missing or the key is not present.
func GetMetadataValue(ctx context.Context, name string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "missing metadata")
	}

	vals := md.Get(name)
	if len(vals) == 0 {
		return "", status.Error(codes.InvalidArgument, fmt.Sprintf("missing metadata key: %s", name))
	}
	return vals[0], nil
}

// HasMetadataKey reports whether incoming gRPC metadata contains the given key.
func HasMetadataKey(ctx context.Context, name string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	return len(md.Get(name)) > 0
}
