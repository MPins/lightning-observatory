// Package graphtest provides a fake graph source and helpers for tests.
package graphtest

import (
	"context"

	"github.com/MPins/lightning-observatory/internal/graph"
)

// Source is a graph.Source that serves a fixed snapshot and then a fixed
// list of updates, closing the stream after the last one.
type Source struct {
	Snap    graph.Snapshot
	Updates []graph.Update
}

var _ graph.Source = (*Source)(nil)

// Subscribe returns a stream with s.Updates.
func (s *Source) Subscribe(ctx context.Context) (<-chan graph.Update, error) {
	ch := make(chan graph.Update)
	go func() {
		defer close(ch)
		for _, u := range s.Updates {
			select {
			case ch <- u:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Snapshot returns s.Snap.
func (s *Source) Snapshot(context.Context) (graph.Snapshot, error) {
	return s.Snap, nil
}

// Key returns a deterministic public key whose last byte is b, for tests
// that need distinct, ordered keys.
func Key(b byte) graph.PubKey {
	var pk graph.PubKey
	pk[0] = 0x02
	pk[32] = b
	return pk
}
