// Package lnd is the phase 1 graph source: it reads the public graph from an
// lnd node over gRPC, with DescribeGraph for the snapshot and
// SubscribeChannelGraph for the changes. A readonly macaroon is enough.
//
// The gRPC client in ./lnrpc is generated from proto/lnrpc/lightning.proto,
// a subset of lnd's, so the observatory does not depend on the lnd module.
package lnd

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/source/lnd/lnrpc"
)

// maxMsgSize is the largest response accepted. The mainnet graph is far
// above gRPC's 4 MB default; lncli uses the same 200 MB.
const maxMsgSize = 200 * 1024 * 1024

// Config says how to reach lnd.
type Config struct {
	// Host is the gRPC address, e.g. "localhost:10009".
	Host string
	// TLSCertPath is lnd's tls.cert.
	TLSCertPath string
	// MacaroonPath is a macaroon file; readonly.macaroon is enough.
	MacaroonPath string
}

// Source reads the graph from lnd. It implements graph.Source.
type Source struct {
	conn   *grpc.ClientConn
	client lnrpc.LightningClient
	// now stamps node updates, which carry no timestamp in lnd's stream.
	now func() time.Time
}

var _ graph.Source = (*Source)(nil)

// Dial prepares the connection to lnd. It does not contact the node until
// the first call.
func Dial(cfg Config) (*Source, error) {
	tlsCreds, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, "")
	if err != nil {
		return nil, fmt.Errorf("tls cert: %w", err)
	}
	mac, err := os.ReadFile(cfg.MacaroonPath)
	if err != nil {
		return nil, fmt.Errorf("macaroon: %w", err)
	}
	conn, err := grpc.NewClient(cfg.Host,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(macaroonCreds(mac)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc client for %s: %w", cfg.Host, err)
	}
	return newSource(conn, lnrpc.NewLightningClient(conn)), nil
}

func newSource(conn *grpc.ClientConn, client lnrpc.LightningClient) *Source {
	return &Source{conn: conn, client: client, now: time.Now}
}

// Close closes the connection.
func (s *Source) Close() error { return s.conn.Close() }

// Snapshot returns the announced graph.
func (s *Source) Snapshot(ctx context.Context) (graph.Snapshot, error) {
	resp, err := s.client.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{})
	if err != nil {
		return graph.Snapshot{}, fmt.Errorf("DescribeGraph: %w", err)
	}
	return convertGraph(resp)
}

// Subscribe streams graph changes. The channel closes when ctx ends or the
// stream fails; a failure is logged.
func (s *Source) Subscribe(ctx context.Context) (<-chan graph.Update, error) {
	stream, err := s.client.SubscribeChannelGraph(ctx, &lnrpc.GraphTopologySubscription{})
	if err != nil {
		return nil, fmt.Errorf("SubscribeChannelGraph: %w", err)
	}
	out := make(chan graph.Update)
	go func() {
		defer close(out)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("lnd graph stream ended", "err", err)
				}
				return
			}
			updates, err := convertTopology(msg, s.now())
			if err != nil {
				slog.Error("lnd graph update", "err", err)
				return
			}
			for _, u := range updates {
				select {
				case out <- u:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// macaroonCreds sends the macaroon with every call, as lnd expects.
type macaroonCreds []byte

func (m macaroonCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"macaroon": hex.EncodeToString(m)}, nil
}

func (macaroonCreds) RequireTransportSecurity() bool { return true }
