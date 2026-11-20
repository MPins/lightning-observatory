package graph

// Tracker holds the current graph and replaces it with a fresh snapshot
// from time to time. lnd's stream does not report every removal (zombie
// channels and orphan nodes are pruned silently) and may deliver
// notifications out of order, so a graph fed only by the stream drifts; a
// periodic snapshot brings it back.
//
// The stream keeps running during a refresh. Updates that arrive while the
// snapshot is being fetched are applied to the current graph and also
// queued, then replayed on the new graph before it replaces the old one, so
// none is lost whether or not the snapshot already had it.
//
// A Tracker is not safe for concurrent use.
type Tracker struct {
	g          *Graph
	refreshing bool
	pending    []Update
}

// NewTracker starts tracking g.
func NewTracker(g *Graph) *Tracker { return &Tracker{g: g} }

// Graph returns the current graph. It changes after FinishRefresh.
func (t *Tracker) Graph() *Graph { return t.g }

// Apply applies an update to the current graph, queueing it during a
// refresh.
func (t *Tracker) Apply(u Update) {
	t.g.Apply(u)
	if t.refreshing {
		t.pending = append(t.pending, u)
	}
}

// Refreshing reports whether a refresh has started and not finished.
func (t *Tracker) Refreshing() bool { return t.refreshing }

// StartRefresh is called just before fetching a snapshot.
func (t *Tracker) StartRefresh() {
	t.refreshing = true
	t.pending = nil
}

// FinishRefresh replaces the graph with snap plus the updates queued since
// StartRefresh.
func (t *Tracker) FinishRefresh(snap Snapshot) {
	g := FromSnapshot(snap)
	for _, u := range t.pending {
		g.Apply(u)
	}
	t.g = g
	t.refreshing = false
	t.pending = nil
}

// AbortRefresh keeps the current graph when the snapshot failed.
func (t *Tracker) AbortRefresh() {
	t.refreshing = false
	t.pending = nil
}
