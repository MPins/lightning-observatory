package lnd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/graph/graphtest"
	"github.com/MPins/lightning-observatory/internal/source/lnd/lnrpc"
)

var (
	ka, kb, kc = graphtest.Key(1), graphtest.Key(2), graphtest.Key(3)
	t0         = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
)

// fakeClient serves a fixed graph and a fixed list of stream messages.
type fakeClient struct {
	lnrpc.LightningClient // nil; only the two methods below are called
	graph                 *lnrpc.ChannelGraph
	stream                []*lnrpc.GraphTopologyUpdate
}

func (f *fakeClient) DescribeGraph(context.Context, *lnrpc.ChannelGraphRequest, ...grpc.CallOption) (*lnrpc.ChannelGraph, error) {
	return f.graph, nil
}

func (f *fakeClient) SubscribeChannelGraph(context.Context, *lnrpc.GraphTopologySubscription, ...grpc.CallOption) (grpc.ServerStreamingClient[lnrpc.GraphTopologyUpdate], error) {
	return &fakeStream{msgs: f.stream}, nil
}

type fakeStream struct {
	grpc.ClientStream // nil; only Recv is called
	msgs              []*lnrpc.GraphTopologyUpdate
}

func (s *fakeStream) Recv() (*lnrpc.GraphTopologyUpdate, error) {
	if len(s.msgs) == 0 {
		return nil, io.EOF
	}
	m := s.msgs[0]
	s.msgs = s.msgs[1:]
	return m, nil
}

func TestSourceLoadsGraph(t *testing.T) {
	client := &fakeClient{
		graph: &lnrpc.ChannelGraph{
			Nodes: []*lnrpc.LightningNode{
				{
					PubKey: ka.String(), Alias: "alice", Color: "#3399ff",
					LastUpdate: uint32(t0.Unix()),
					Addresses:  []*lnrpc.NodeAddress{{Network: "tcp", Addr: "203.0.113.1:9735"}},
					// 729 is unknown to lnd but must still arrive.
					Features: map[uint32]*lnrpc.Feature{31: {}, 2023: {}, 729: {IsKnown: false}},
				},
				{PubKey: kb.String(), Alias: "bob"},
			},
			Edges: []*lnrpc.ChannelEdge{{
				ChannelId: 1 << 40, Node1Pub: ka.String(), Node2Pub: kb.String(), Capacity: 1_000_000,
				Node1Policy: &lnrpc.RoutingPolicy{TimeLockDelta: 80, FeeBaseMsat: 1000, FeeRateMilliMsat: 1,
					MinHtlc: 1000, MaxHtlcMsat: 990_000_000, LastUpdate: uint32(t0.Unix())},
			}},
		},
		stream: []*lnrpc.GraphTopologyUpdate{{
			NodeUpdates: []*lnrpc.NodeUpdate{{IdentityKey: kc.String(), Alias: "carol", Color: "#49daaa"}},
			ChannelUpdates: []*lnrpc.ChannelEdgeUpdate{{
				ChanId: 2, Capacity: 500_000, AdvertisingNode: kc.String(), ConnectingNode: kb.String(),
				RoutingPolicy: &lnrpc.RoutingPolicy{TimeLockDelta: 144, FeeRateMilliMsat: 200, Disabled: true},
			}},
			ClosedChans: []*lnrpc.ClosedChannelUpdate{{ChanId: 1 << 40}},
		}},
	}
	src := newSource(nil, client)
	src.now = func() time.Time { return t0 }

	g, updates, err := graph.Load(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	alice, ok := g.Node(ka)
	if !ok || alice.Alias != "alice" || alice.Color != [3]byte{0x33, 0x99, 0xff} {
		t.Fatalf("alice = %+v", alice)
	}
	if !alice.Features.Has(729) || !alice.Features.Has(2023) || !alice.LastUpdate.Equal(t0) {
		t.Fatalf("alice features/last update = %v / %v", alice.Features, alice.LastUpdate)
	}
	if len(alice.Addresses) != 1 || alice.Addresses[0] != "203.0.113.1:9735" {
		t.Fatalf("alice addresses = %v", alice.Addresses)
	}
	ch, _ := g.Channel(1 << 40)
	want := graph.Policy{TimeLockDelta: 80, FeeBaseMsat: 1000, FeeRatePPM: 1,
		MinHTLCMsat: 1000, MaxHTLCMsat: 990_000_000, LastUpdate: t0}
	if ch.Node1Policy == nil || *ch.Node1Policy != want || ch.Node2Policy != nil {
		t.Fatalf("channel policies = %+v / %+v", ch.Node1Policy, ch.Node2Policy)
	}

	for u := range updates {
		g.Apply(u)
	}

	if g.NodeCount() != 3 || g.ChannelCount() != 1 {
		t.Fatalf("after stream: %d nodes, %d channels; want 3, 1", g.NodeCount(), g.ChannelCount())
	}
	carol, _ := g.Node(kc)
	if carol.Alias != "carol" || !carol.LastUpdate.Equal(t0) {
		t.Fatalf("carol = %+v", carol)
	}
	ch2, ok := g.Channel(2)
	// b sorts before c, so carol's policy is node2's.
	if !ok || ch2.Node1 != kb || ch2.Node2Policy == nil || !ch2.Node2Policy.Disabled {
		t.Fatalf("channel 2 = %+v", ch2)
	}
}

func TestBadPubKeyFailsSnapshot(t *testing.T) {
	src := newSource(nil, &fakeClient{graph: &lnrpc.ChannelGraph{
		Nodes: []*lnrpc.LightningNode{{PubKey: "zz"}},
	}})
	if _, err := src.Snapshot(context.Background()); err == nil || !strings.Contains(err.Error(), "zz") {
		t.Fatalf("Snapshot() error = %v, want one naming the bad key", err)
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string][3]byte{
		"#3399ff": {0x33, 0x99, 0xff},
		"49daaa":  {0x49, 0xda, 0xaa},
		"":        {},
		"#12345":  {},
		"#zzzzzz": {},
	} {
		if got := parseColor(in); got != want {
			t.Errorf("parseColor(%q) = %x, want %x", in, got, want)
		}
	}
}
