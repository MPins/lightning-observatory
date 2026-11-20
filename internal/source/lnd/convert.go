package lnd

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/source/lnd/lnrpc"
)

func convertGraph(g *lnrpc.ChannelGraph) (graph.Snapshot, error) {
	var snap graph.Snapshot
	for _, n := range g.GetNodes() {
		node, err := convertNode(n)
		if err != nil {
			return graph.Snapshot{}, err
		}
		snap.Nodes = append(snap.Nodes, node)
	}
	for _, e := range g.GetEdges() {
		ch, err := convertEdge(e)
		if err != nil {
			return graph.Snapshot{}, err
		}
		snap.Channels = append(snap.Channels, ch)
	}
	return snap, nil
}

func convertNode(n *lnrpc.LightningNode) (graph.Node, error) {
	pk, err := graph.ParsePubKey(n.GetPubKey())
	if err != nil {
		return graph.Node{}, err
	}
	return graph.Node{
		PubKey:     pk,
		Alias:      n.GetAlias(),
		Color:      parseColor(n.GetColor()),
		Addresses:  addresses(n.GetAddresses()),
		Features:   features(n.GetFeatures()),
		LastUpdate: unix(n.GetLastUpdate()),
	}, nil
}

func convertEdge(e *lnrpc.ChannelEdge) (graph.Channel, error) {
	n1, err := graph.ParsePubKey(e.GetNode1Pub())
	if err != nil {
		return graph.Channel{}, fmt.Errorf("channel %d: %w", e.GetChannelId(), err)
	}
	n2, err := graph.ParsePubKey(e.GetNode2Pub())
	if err != nil {
		return graph.Channel{}, fmt.Errorf("channel %d: %w", e.GetChannelId(), err)
	}
	return graph.Channel{
		ID:          e.GetChannelId(),
		Node1:       n1,
		Node2:       n2,
		CapacitySat: e.GetCapacity(),
		Node1Policy: convertPolicy(e.GetNode1Policy()),
		Node2Policy: convertPolicy(e.GetNode2Policy()),
	}, nil
}

// convertPolicy returns nil for a direction lnd has no update for.
func convertPolicy(p *lnrpc.RoutingPolicy) *graph.Policy {
	if p == nil {
		return nil
	}
	return &graph.Policy{
		TimeLockDelta: uint16(p.GetTimeLockDelta()),
		MinHTLCMsat:   uint64(p.GetMinHtlc()),
		MaxHTLCMsat:   p.GetMaxHtlcMsat(),
		FeeBaseMsat:   uint64(p.GetFeeBaseMsat()),
		FeeRatePPM:    uint32(p.GetFeeRateMilliMsat()),
		Disabled:      p.GetDisabled(),
		LastUpdate:    unix(p.GetLastUpdate()),
	}
}

// convertTopology turns one stream message into updates: nodes first, then
// policies, then closures, as lnd groups them. now stamps node updates.
func convertTopology(u *lnrpc.GraphTopologyUpdate, now time.Time) ([]graph.Update, error) {
	var out []graph.Update
	for _, n := range u.GetNodeUpdates() {
		pk, err := graph.ParsePubKey(n.GetIdentityKey())
		if err != nil {
			return nil, err
		}
		out = append(out, graph.NodeUpdate{Node: graph.Node{
			PubKey:     pk,
			Alias:      n.GetAlias(),
			Color:      parseColor(n.GetColor()),
			Addresses:  addresses(n.GetNodeAddresses()),
			Features:   features(n.GetFeatures()),
			LastUpdate: now,
		}})
	}
	for _, c := range u.GetChannelUpdates() {
		p := convertPolicy(c.GetRoutingPolicy())
		if p == nil {
			continue
		}
		from, err := graph.ParsePubKey(c.GetAdvertisingNode())
		if err != nil {
			return nil, fmt.Errorf("channel %d: %w", c.GetChanId(), err)
		}
		to, err := graph.ParsePubKey(c.GetConnectingNode())
		if err != nil {
			return nil, fmt.Errorf("channel %d: %w", c.GetChanId(), err)
		}
		out = append(out, graph.PolicyUpdate{
			ChannelID:   c.GetChanId(),
			From:        from,
			To:          to,
			CapacitySat: c.GetCapacity(),
			Policy:      *p,
		})
	}
	for _, c := range u.GetClosedChans() {
		out = append(out, graph.ChannelClosed{ChannelID: c.GetChanId()})
	}
	return out, nil
}

// parseColor reads "#rrggbb"; anything else gives black.
func parseColor(s string) [3]byte {
	var c [3]byte
	b, err := hex.DecodeString(strings.TrimPrefix(s, "#"))
	if err == nil && len(b) == 3 {
		copy(c[:], b)
	}
	return c
}

func addresses(as []*lnrpc.NodeAddress) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.GetAddr())
	}
	return out
}

// features keeps every bit lnd returns, unknown ones included (e.g. 729).
func features(fs map[uint32]*lnrpc.Feature) graph.Features {
	f := make(graph.Features, len(fs))
	for bit := range fs {
		f[bit] = struct{}{}
	}
	return f
}

func unix(sec uint32) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(int64(sec), 0).UTC()
}
