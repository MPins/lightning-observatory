package graph_test

import (
	"testing"
	"time"

	"github.com/MPins/lightning-observatory/internal/graph"
)

func TestOlderPolicyDropped(t *testing.T) {
	g := graph.New()
	newer := graph.Policy{FeeRatePPM: 2, LastUpdate: t0.Add(time.Hour)}
	g.Apply(graph.PolicyUpdate{ChannelID: 1, From: a, To: b, Policy: newer})

	// An older notification arriving late must not win.
	g.Apply(graph.PolicyUpdate{ChannelID: 1, From: a, To: b,
		Policy: graph.Policy{FeeRatePPM: 1, LastUpdate: t0}})
	if ch, _ := g.Channel(1); ch.Node1Policy.FeeRatePPM != 2 {
		t.Fatalf("fee = %d, want the newer 2", ch.Node1Policy.FeeRatePPM)
	}

	// Same timestamp applies (lnd re-sends what it already has).
	g.Apply(graph.PolicyUpdate{ChannelID: 1, From: a, To: b,
		Policy: graph.Policy{FeeRatePPM: 3, LastUpdate: newer.LastUpdate}})
	if ch, _ := g.Channel(1); ch.Node1Policy.FeeRatePPM != 3 {
		t.Fatalf("fee = %d, want 3", ch.Node1Policy.FeeRatePPM)
	}

	// The other direction has its own timestamp.
	g.Apply(graph.PolicyUpdate{ChannelID: 1, From: b, To: a,
		Policy: graph.Policy{FeeRatePPM: 9, LastUpdate: t0}})
	if ch, _ := g.Channel(1); ch.Node2Policy == nil || ch.Node2Policy.FeeRatePPM != 9 {
		t.Fatalf("node2 policy = %+v", ch.Node2Policy)
	}
}

func TestTrackerRefresh(t *testing.T) {
	// Before the refresh: a-b (later pruned as a zombie by lnd, without
	// notice) and b-c.
	tr := graph.NewTracker(graph.FromSnapshot(graph.Snapshot{
		Nodes: []graph.Node{{PubKey: a}, {PubKey: b}, {PubKey: c}},
		Channels: []graph.Channel{
			{ID: 1, Node1: a, Node2: b},
			{ID: 2, Node1: b, Node2: c},
		},
	}))

	tr.StartRefresh()
	if !tr.Refreshing() {
		t.Fatal("not refreshing after StartRefresh")
	}
	// While the snapshot is fetched, c-d opens. The snapshot below was
	// taken before it, so only the queue can bring it into the new graph.
	open := graph.PolicyUpdate{ChannelID: 3, From: c, To: d, CapacitySat: 7,
		Policy: graph.Policy{LastUpdate: t0}}
	tr.Apply(open)
	if _, ok := tr.Graph().Channel(3); !ok {
		t.Fatal("update during refresh not applied to the current graph")
	}

	tr.FinishRefresh(graph.Snapshot{
		Nodes:    []graph.Node{{PubKey: b}, {PubKey: c}},
		Channels: []graph.Channel{{ID: 2, Node1: b, Node2: c}},
	})

	g := tr.Graph()
	if tr.Refreshing() {
		t.Fatal("still refreshing after FinishRefresh")
	}
	if _, ok := g.Channel(1); ok {
		t.Fatal("zombie channel 1 survived the refresh")
	}
	if _, ok := g.Node(a); ok {
		t.Fatal("orphan node a survived the refresh")
	}
	if ch, ok := g.Channel(3); !ok || ch.CapacitySat != 7 {
		t.Fatalf("channel 3 opened during the refresh = %+v, %t", ch, ok)
	}
	if got := channelIDs(g, c); len(got) != 2 {
		t.Fatalf("channels of c = %v, want [2 3]", got)
	}

	// After the refresh, updates are no longer queued for the next one.
	tr.Apply(graph.ChannelClosed{ChannelID: 2})
	tr.StartRefresh()
	tr.FinishRefresh(graph.Snapshot{Channels: []graph.Channel{{ID: 2, Node1: b, Node2: c}}})
	if _, ok := tr.Graph().Channel(2); !ok {
		t.Fatal("an update from before StartRefresh was replayed")
	}
}

func TestTrackerAbortKeepsGraph(t *testing.T) {
	tr := graph.NewTracker(graph.FromSnapshot(graph.Snapshot{
		Channels: []graph.Channel{{ID: 1, Node1: a, Node2: b}},
	}))
	before := tr.Graph()
	tr.StartRefresh()
	tr.Apply(graph.ChannelClosed{ChannelID: 1})
	tr.AbortRefresh()

	if tr.Graph() != before || tr.Refreshing() {
		t.Fatal("abort replaced the graph or kept refreshing")
	}
	if _, ok := tr.Graph().Channel(1); ok {
		t.Fatal("update applied during the failed refresh was lost")
	}
}
