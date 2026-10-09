package runneridentity

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	pb "github.com/petarnenov/bot-space/api/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// bearer is owned by the supervisor. It cannot send credentials over plaintext
// and has no exported field for accidental JSON status serialization.
type bearer struct{ token string }

func (b bearer) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}
func (b bearer) RequireTransportSecurity() bool { return true }

// DialNative creates a verified TLS native connection using trust metadata
// supplied by authenticated HTTPS enrollment. Callers must close and recreate
// it after credential rotation, replaying their committed durable cursor.
func DialNative(lease Lease) (*grpc.ClientConn, pb.ControlClient, error) {
	if err := lease.Validate(); err != nil {
		return nil, nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(lease.CA)) {
		return nil, nil, ErrInvalid
	}
	conn, err := grpc.NewClient(lease.Endpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})), grpc.WithPerRPCCredentials(bearer{lease.Token}), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(256<<10), grpc.MaxCallSendMsgSize(256<<10)))
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	return conn, pb.NewControlClient(conn), nil
}
