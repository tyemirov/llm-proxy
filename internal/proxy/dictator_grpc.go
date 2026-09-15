package proxy

import (
	"context"
	"crypto/tls"
	"fmt"

	dictator "github.com/tyemirov/dictator/sdk/go/dictatorspeechv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	dictatorAddressField = "grpc_address"
	dictatorTokenField   = "grpc_auth_token"
	dictatorTLSField     = "grpc_tls"
)

func openDictatorConnection(ctx context.Context, values map[string]string) (*grpc.ClientConn, context.Context, error) {
	var transport credentials.TransportCredentials
	switch values[dictatorTLSField] {
	case "true":
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	case "false":
		transport = insecure.NewCredentials()
	default:
		return nil, ctx, fmt.Errorf("dictator TLS setting: %w", errManagedConnectionInvalid)
	}
	connection, err := grpc.NewClient(values[dictatorAddressField], grpc.WithTransportCredentials(transport))
	if err != nil {
		return nil, ctx, fmt.Errorf("open Dictator connection: %w", err)
	}
	return connection, metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+values[dictatorTokenField]), nil
}

func verifyDictatorConnection(ctx context.Context, values map[string]string) error {
	connection, callContext, err := openDictatorConnection(ctx, values)
	if err != nil {
		return errProviderKeyVerificationUnavailable
	}
	defer connection.Close()
	_, err = dictator.NewVoiceServiceClient(connection).ListSynthesisVoices(callContext, &dictator.ListSynthesisVoicesRequest{})
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.Unauthenticated, codes.PermissionDenied:
		return errProviderKeyRejected
	case codes.ResourceExhausted:
		return errProviderKeyVerificationRateLimited
	case codes.DeadlineExceeded, codes.Canceled:
		return errProviderKeyVerificationTimedOut
	default:
		return errProviderKeyVerificationUnavailable
	}
}
