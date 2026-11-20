// Package graph holds the in-memory model of the public Lightning graph and
// the interface through which a source feeds it. The classifier and the
// metrics read only this package; they never know which source filled it.
package graph

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"iter"
	"maps"
	"time"
)

// PubKey is a node's compressed public key.
type PubKey [33]byte

// ParsePubKey decodes a hex-encoded compressed public key.
func ParsePubKey(s string) (PubKey, error) {
	var pk PubKey
	b, err := hex.DecodeString(s)
	if err != nil {
		return pk, fmt.Errorf("pubkey %q: %w", s, err)
	}
	if len(b) != len(pk) {
		return pk, fmt.Errorf("pubkey %q: %d bytes, want %d", s, len(b), len(pk))
	}
	copy(pk[:], b)
	return pk, nil
}

func (pk PubKey) String() string { return hex.EncodeToString(pk[:]) }

// Features is the set of feature bits a node announces.
type Features map[uint32]struct{}

// NewFeatures returns a set with the given bits.
func NewFeatures(bits ...uint32) Features {
	f := make(Features, len(bits))
	for _, b := range bits {
		f[b] = struct{}{}
	}
	return f
}

// Has reports whether bit is set.
func (f Features) Has(bit uint32) bool {
	_, ok := f[bit]
	return ok
}

// Node is what a node_announcement says about a node.
type Node struct {
	PubKey PubKey
	Alias  string
	// Color is the announced RGB color.
	Color [3]byte
	// Addresses are "host:port" strings, including .onion hosts.
	Addresses []string
	Features  Features
	// LastUpdate is the announcement timestamp, or the time the source saw
	// the update when the source does not carry one.
	LastUpdate time.Time
}

// Policy is one direction of a channel, from one channel_update.
type Policy struct {
	TimeLockDelta uint16
	MinHTLCMsat   uint64
	MaxHTLCMsat   uint64
	FeeBaseMsat   uint64
	// FeeRatePPM is the proportional fee in millionths.
	FeeRatePPM uint32
	Disabled   bool
	LastUpdate time.Time
}

// Channel is an announced channel. Node1 sorts before Node2, as in BOLT 7;
// Node1Policy is the policy Node1 announced for forwarding to Node2.
type Channel struct {
	ID           uint64 // short channel ID
	Node1, Node2 PubKey
	CapacitySat  int64
	Node1Policy  *Policy
	Node2Policy  *Policy
}

// Graph is the current view of the network. It is not safe for concurrent
// use.
type Graph struct {
	nodes    map[PubKey]Node
	channels map[uint64]Channel
	byNode   map[PubKey]map[uint64]struct{}
}

// New returns an empty graph.
func New() *Graph {
	return &Graph{
		nodes:    make(map[PubKey]Node),
		channels: make(map[uint64]Channel),
		byNode:   make(map[PubKey]map[uint64]struct{}),
	}
}

// NodeCount returns the number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// ChannelCount returns the number of channels.
func (g *Graph) ChannelCount() int { return len(g.channels) }

// Node returns the node with the given key.
func (g *Graph) Node(pk PubKey) (Node, bool) {
	n, ok := g.nodes[pk]
	return n, ok
}

// Channel returns the channel with the given ID.
func (g *Graph) Channel(id uint64) (Channel, bool) {
	c, ok := g.channels[id]
	return c, ok
}

// Nodes iterates over all nodes in no particular order.
func (g *Graph) Nodes() iter.Seq[Node] { return maps.Values(g.nodes) }

// Channels iterates over all channels in no particular order.
func (g *Graph) Channels() iter.Seq[Channel] { return maps.Values(g.channels) }

// ChannelsOf iterates over the channels of one node.
func (g *Graph) ChannelsOf(pk PubKey) iter.Seq[Channel] {
	return func(yield func(Channel) bool) {
		for id := range g.byNode[pk] {
			if !yield(g.channels[id]) {
				return
			}
		}
	}
}

// Update is one change from a source: NodeUpdate, PolicyUpdate or
// ChannelClosed.
type Update interface{ isUpdate() }

// NodeUpdate replaces what is known about a node.
type NodeUpdate struct{ Node Node }

// PolicyUpdate sets the policy that From announced for channel ChannelID.
// It creates the channel if the graph does not have it yet.
type PolicyUpdate struct {
	ChannelID   uint64
	From, To    PubKey
	CapacitySat int64
	Policy      Policy
}

// ChannelClosed removes a channel.
type ChannelClosed struct{ ChannelID uint64 }

func (NodeUpdate) isUpdate()    {}
func (PolicyUpdate) isUpdate()  {}
func (ChannelClosed) isUpdate() {}

// AddNode adds or replaces a node.
func (g *Graph) AddNode(n Node) { g.nodes[n.PubKey] = n }

// AddChannel adds or replaces a channel. The nodes are put in BOLT 7 order,
// swapping the policies with them if needed.
func (g *Graph) AddChannel(c Channel) {
	if bytes.Compare(c.Node1[:], c.Node2[:]) > 0 {
		c.Node1, c.Node2 = c.Node2, c.Node1
		c.Node1Policy, c.Node2Policy = c.Node2Policy, c.Node1Policy
	}
	g.channels[c.ID] = c
	g.index(c.Node1, c.ID)
	g.index(c.Node2, c.ID)
}

// Apply applies one update. A policy older than the one stored is dropped,
// because lnd may deliver notifications out of order. Node updates carry no
// timestamp in lnd's stream, so they always apply; the periodic snapshot
// (Tracker) corrects any that arrived out of order.
func (g *Graph) Apply(u Update) {
	switch u := u.(type) {
	case NodeUpdate:
		g.AddNode(u.Node)

	case PolicyUpdate:
		c, ok := g.channels[u.ChannelID]
		if !ok {
			c = Channel{ID: u.ChannelID, Node1: u.From, Node2: u.To}
		}
		var slot **Policy
		switch u.From {
		case c.Node1:
			slot = &c.Node1Policy
		case c.Node2:
			slot = &c.Node2Policy
		default:
			// The update names a node that is not on this channel.
			return
		}
		if old := *slot; old != nil && u.Policy.LastUpdate.Before(old.LastUpdate) {
			return
		}
		p := u.Policy
		*slot = &p
		if u.CapacitySat != 0 {
			c.CapacitySat = u.CapacitySat
		}
		g.AddChannel(c)

	case ChannelClosed:
		c, ok := g.channels[u.ChannelID]
		if !ok {
			return
		}
		delete(g.channels, u.ChannelID)
		g.unindex(c.Node1, c.ID)
		g.unindex(c.Node2, c.ID)
	}
}

func (g *Graph) index(pk PubKey, id uint64) {
	set, ok := g.byNode[pk]
	if !ok {
		set = make(map[uint64]struct{})
		g.byNode[pk] = set
	}
	set[id] = struct{}{}
}

func (g *Graph) unindex(pk PubKey, id uint64) {
	set := g.byNode[pk]
	delete(set, id)
	if len(set) == 0 {
		delete(g.byNode, pk)
	}
}
