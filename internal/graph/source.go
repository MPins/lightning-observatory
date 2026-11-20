package graph

import (
	"context"
	"fmt"
)

// Snapshot is the whole graph as a source sees it at one moment.
type Snapshot struct {
	Nodes    []Node
	Channels []Channel
}

// Source delivers the graph: a snapshot at startup, then a stream of
// changes. lnd's gRPC API is the phase 1 source; the gossip source of
// phase 3 implements the same interface.
type Source interface {
	// Subscribe starts the stream of changes. The channel is closed when
	// ctx ends or the source fails.
	Subscribe(ctx context.Context) (<-chan Update, error)

	// Snapshot returns the full graph.
	Snapshot(ctx context.Context) (Snapshot, error)
}

// Load subscribes to src, then takes its snapshot and builds the graph.
// Subscribing first means no change between the two calls is lost; changes
// already in the snapshot are applied again, which leaves the same state.
// The caller applies the returned stream with Graph.Apply.
func Load(ctx context.Context, src Source) (*Graph, <-chan Update, error) {
	updates, err := src.Subscribe(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("subscribe: %w", err)
	}
	snap, err := src.Snapshot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot: %w", err)
	}
	return FromSnapshot(snap), updates, nil
}

// FromSnapshot builds a graph from a snapshot.
func FromSnapshot(snap Snapshot) *Graph {
	g := New()
	for _, n := range snap.Nodes {
		g.AddNode(n)
	}
	for _, c := range snap.Channels {
		g.AddChannel(c)
	}
	return g
}
