package graph_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/graph/graphtest"
)

var (
	a, b, c, d = graphtest.Key(1), graphtest.Key(2), graphtest.Key(3), graphtest.Key(4)
	t0         = time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
)

func TestLoadFromFakeSource(t *testing.T) {
	src := &graphtest.Source{
		Snap: graph.Snapshot{
			Nodes: []graph.Node{
				{PubKey: a, Alias: "a", Features: graph.NewFeatures(31, 2023)},
				{PubKey: b, Alias: "b"},
				{PubKey: c, Alias: "c"},
			},
			Channels: []graph.Channel{
				{ID: 10, Node1: a, Node2: b, CapacitySat: 1_000_000,
					Node1Policy: &graph.Policy{TimeLockDelta: 80, FeeRatePPM: 1}},
				{ID: 11, Node1: b, Node2: c, CapacitySat: 2_000_000},
			},
		},
		Updates: []graph.Update{
			graph.NodeUpdate{Node: graph.Node{PubKey: d, Alias: "d", LastUpdate: t0}},
			// New channel c-d, first seen through d's policy.
			graph.PolicyUpdate{ChannelID: 12, From: d, To: c, CapacitySat: 500_000,
				Policy: graph.Policy{TimeLockDelta: 144, FeeRatePPM: 200}},
			// b now disables its side of 10.
			graph.PolicyUpdate{ChannelID: 10, From: b, To: a,
				Policy: graph.Policy{TimeLockDelta: 34, Disabled: true}},
			graph.ChannelClosed{ChannelID: 11},
			// a renames itself; the last update wins.
			graph.NodeUpdate{Node: graph.Node{PubKey: a, Alias: "a2"}},
		},
	}

	g, updates, err := graph.Load(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if g.NodeCount() != 3 || g.ChannelCount() != 2 {
		t.Fatalf("after snapshot: %d nodes, %d channels; want 3, 2",
			g.NodeCount(), g.ChannelCount())
	}
	if n, _ := g.Node(a); !n.Features.Has(2023) || n.Features.Has(11) {
		t.Fatalf("node a features = %v", n.Features)
	}

	for u := range updates {
		g.Apply(u)
	}

	if g.NodeCount() != 4 || g.ChannelCount() != 2 {
		t.Fatalf("after updates: %d nodes, %d channels; want 4, 2",
			g.NodeCount(), g.ChannelCount())
	}
	if n, _ := g.Node(a); n.Alias != "a2" {
		t.Fatalf("node a alias = %q, want a2", n.Alias)
	}

	ch10, _ := g.Channel(10)
	if ch10.Node1Policy == nil || ch10.Node1Policy.TimeLockDelta != 80 {
		t.Fatalf("channel 10 node1 policy = %+v, want delta 80 kept", ch10.Node1Policy)
	}
	if ch10.Node2Policy == nil || !ch10.Node2Policy.Disabled {
		t.Fatalf("channel 10 node2 policy = %+v, want disabled", ch10.Node2Policy)
	}
	if ch10.CapacitySat != 1_000_000 {
		t.Fatalf("channel 10 capacity = %d, want unchanged", ch10.CapacitySat)
	}

	// Channel 12 was created from d's update; c sorts before d, so d's
	// policy is the node2 policy.
	ch12, ok := g.Channel(12)
	if !ok || ch12.Node1 != c || ch12.Node2 != d || ch12.CapacitySat != 500_000 {
		t.Fatalf("channel 12 = %+v", ch12)
	}
	if ch12.Node1Policy != nil || ch12.Node2Policy == nil || ch12.Node2Policy.FeeRatePPM != 200 {
		t.Fatalf("channel 12 policies = %+v / %+v", ch12.Node1Policy, ch12.Node2Policy)
	}

	if _, ok := g.Channel(11); ok {
		t.Fatal("channel 11 still present after close")
	}
	if got := channelIDs(g, b); !slices.Equal(got, []uint64{10}) {
		t.Fatalf("channels of b = %v, want [10]", got)
	}
	if got := channelIDs(g, c); !slices.Equal(got, []uint64{12}) {
		t.Fatalf("channels of c = %v, want [12]", got)
	}
}

func TestAddChannelOrdersNodes(t *testing.T) {
	g := graph.New()
	g.AddChannel(graph.Channel{ID: 1, Node1: b, Node2: a,
		Node1Policy: &graph.Policy{FeeRatePPM: 7}})

	ch, _ := g.Channel(1)
	if ch.Node1 != a || ch.Node2 != b {
		t.Fatalf("nodes = %v, %v; want a, b", ch.Node1, ch.Node2)
	}
	if ch.Node1Policy != nil || ch.Node2Policy == nil || ch.Node2Policy.FeeRatePPM != 7 {
		t.Fatal("policy did not move with its node")
	}
}

func TestPolicyFromStrangerIgnored(t *testing.T) {
	g := graph.New()
	g.AddChannel(graph.Channel{ID: 1, Node1: a, Node2: b})
	g.Apply(graph.PolicyUpdate{ChannelID: 1, From: c, To: a})

	ch, _ := g.Channel(1)
	if ch.Node1Policy != nil || ch.Node2Policy != nil {
		t.Fatalf("policy from a node not on the channel was applied: %+v", ch)
	}
	if channelIDs(g, c) != nil {
		t.Fatal("stranger was indexed")
	}
}

func TestParsePubKey(t *testing.T) {
	good := "02" + strings.Repeat("00", 31) + "01"
	pk, err := graph.ParsePubKey(good)
	if err != nil || pk != a || pk.String() != good {
		t.Fatalf("ParsePubKey(%q) = %v, %v", good, pk, err)
	}
	for _, bad := range []string{"", "zz", good[:64], good + "00"} {
		if _, err := graph.ParsePubKey(bad); err == nil {
			t.Errorf("ParsePubKey(%q) accepted", bad)
		}
	}
}

func channelIDs(g *graph.Graph, pk graph.PubKey) []uint64 {
	var ids []uint64
	for ch := range g.ChannelsOf(pk) {
		ids = append(ids, ch.ID)
	}
	slices.Sort(ids)
	return ids
}
