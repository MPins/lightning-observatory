package classify

import (
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/MPins/lightning-observatory/internal/graph"
	"github.com/MPins/lightning-observatory/internal/graph/graphtest"
)

// Default feature sets from docs/fingerprints.md.
var (
	lndBits     = []uint32{0, 5, 7, 8, 12, 14, 17, 23, 25, 27, 31, 35, 39, 44, 261, 2023}
	clnBits     = []uint32{0, 5, 7, 8, 11, 12, 14, 17, 19, 23, 25, 27, 35, 39, 43, 44, 47, 51, 55, 63}
	eclairBits  = []uint32{0, 7, 8, 11, 12, 14, 17, 19, 23, 25, 27, 29, 35, 37, 39, 44, 47, 61, 63, 81}
	ldkBits     = []uint32{0, 5, 7, 8, 12, 14, 17, 19, 25, 27, 35, 39, 43, 44, 47, 51, 55, 63}
	ldkNodeBits = append(slices.Clone(ldkBits), 23)
	// Bits every implementation announces; matches no fingerprint.
	commonBits = []uint32{0, 8, 12, 14, 17, 25, 27, 35, 44}
)

// node is the node under test; peer is the other side of its channels.
var (
	self = graphtest.Key(0x10)
	peer = graphtest.Key(0x20)
)

func pol(delta uint16, ppm uint32) *graph.Policy {
	return &graph.Policy{TimeLockDelta: delta, FeeBaseMsat: 1000, FeeRatePPM: ppm}
}

// channels builds one channel per policy, with self as node1 and the peer
// announcing a policy that must be ignored.
func channels(policies ...*graph.Policy) []graph.Channel {
	var chs []graph.Channel
	for i, p := range policies {
		chs = append(chs, graph.Channel{
			ID: uint64(i + 1), Node1: self, Node2: peer,
			Node1Policy: p,
			Node2Policy: pol(9, 9999),
		})
	}
	return chs
}

func TestClassify(t *testing.T) {
	clnColor := [3]byte{self[0], self[1], self[2]}
	lndAlias := hex.EncodeToString(self[:10])

	tests := []struct {
		name     string
		node     graph.Node
		chans    []graph.Channel
		impl     Implementation
		layer    Layer
		conflict bool
		evidence []string // substrings that must appear
	}{
		{
			name:  "lnd default",
			node:  graph.Node{Features: graph.NewFeatures(lndBits...), Color: [3]byte{0x33, 0x99, 0xff}, Alias: lndAlias},
			chans: channels(pol(80, 1), pol(80, 1)),
			impl:  LND, layer: LayerFeatures,
			evidence: []string{"bit 31", "(80, 1 ppm) on 2 of 2", "#3399ff", "first 10 bytes"},
		},
		{
			name:  "CLN default",
			node:  graph.Node{Features: graph.NewFeatures(clnBits...), Color: clnColor},
			chans: channels(pol(34, 10)),
			impl:  CLN, layer: LayerFeatures,
			evidence: []string{"bit 11", "CLN mainnet", "first 3 bytes"},
		},
		{
			name:  "CLN on regtest",
			node:  graph.Node{Features: graph.NewFeatures(clnBits...)},
			chans: channels(pol(6, 10)),
			impl:  CLN, layer: LayerFeatures,
			evidence: []string{"CLN test-network"},
		},
		{
			name:  "Eclair default",
			node:  graph.Node{Features: graph.NewFeatures(eclairBits...), Color: [3]byte{0x49, 0xda, 0xaa}, Alias: "eclair"},
			chans: channels(pol(144, 200)),
			impl:  Eclair, layer: LayerFeatures,
			evidence: []string{"bit 37", "Eclair default", "#49daaa", `"eclair"`},
		},
		{
			name:  "LDK default",
			node:  graph.Node{Features: graph.NewFeatures(ldkBits...), Alias: "my-app"},
			chans: channels(pol(72, 0)),
			impl:  LDK, layer: LayerFeatures,
			evidence: []string{"bits 43, 47, 63", "LDK default"},
		},
		{
			name:  "ldk-node default",
			node:  graph.Node{Features: graph.NewFeatures(ldkNodeBits...), Alias: "node"},
			chans: channels(pol(72, 0)),
			impl:  LDK, layer: LayerFeatures,
			evidence: []string{"ldk-node default", "#000000"},
		},
		{
			name:  "lnd with custom fees and policy still found by bits",
			node:  graph.Node{Features: graph.NewFeatures(lndBits...), Alias: "routing"},
			chans: channels(pol(40, 500)),
			impl:  LND, layer: LayerFeatures,
		},
		{
			name:  "features beat a conflicting policy",
			node:  graph.Node{Features: graph.NewFeatures(lndBits...)},
			chans: channels(pol(144, 200)),
			impl:  LND, layer: LayerFeatures, conflict: true,
			evidence: []string{"bit 31", "Eclair default"},
		},
		{
			name:  "policy beats a conflicting color",
			node:  graph.Node{Features: graph.NewFeatures(commonBits...), Color: [3]byte{0x49, 0xda, 0xaa}},
			chans: channels(pol(80, 1)),
			impl:  LND, layer: LayerPolicy, conflict: true,
			evidence: []string{"lnd default", "#49daaa"},
		},
		{
			name:  "policy decides without fingerprint bits",
			node:  graph.Node{Features: graph.NewFeatures(commonBits...)},
			chans: channels(pol(144, 200), pol(144, 200), pol(80, 1)),
			impl:  Eclair, layer: LayerPolicy,
			evidence: []string{"(144, 200 ppm) on 2 of 3", "(80, 1 ppm) on 1 of 3"},
		},
		{
			name:  "policy tie decides nothing",
			node:  graph.Node{Features: graph.NewFeatures(commonBits...)},
			chans: channels(pol(144, 200), pol(80, 1)),
			impl:  Unknown, layer: LayerNone,
		},
		{
			name:  "both CLN profiles add up, no tie",
			node:  graph.Node{Features: graph.NewFeatures(commonBits...)},
			chans: channels(pol(34, 10), pol(6, 10), pol(72, 0)),
			impl:  CLN, layer: LayerPolicy,
		},
		{
			name: "announcement decides last",
			node: graph.Node{Features: graph.NewFeatures(commonBits...), Color: clnColor},
			impl: CLN, layer: LayerAnnouncement,
		},
		{
			name: "lnd alias without anything else",
			node: graph.Node{Alias: lndAlias},
			impl: LND, layer: LayerAnnouncement,
		},
		{
			name: "black color alone is not enough",
			node: graph.Node{Features: graph.NewFeatures(commonBits...), Alias: "x"},
			impl: Unknown, layer: LayerNone,
			evidence: []string{"#000000"},
		},
		{
			name:  "nothing matches",
			node:  graph.Node{Features: graph.NewFeatures(commonBits...), Color: [3]byte{1, 2, 3}, Alias: "custom"},
			chans: channels(pol(40, 50), nil),
			impl:  Unknown, layer: LayerNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.node.PubKey = self
			got := Classify(tc.node, slices.Values(tc.chans))
			if got.Impl != tc.impl || got.Layer != tc.layer || got.Conflict != tc.conflict {
				t.Fatalf("Classify() = %s by %s (conflict %t), want %s by %s (conflict %t)\nevidence: %q",
					got.Impl, got.Layer, got.Conflict, tc.impl, tc.layer, tc.conflict, got.Evidence)
			}
			all := strings.Join(got.Evidence, "\n")
			for _, want := range tc.evidence {
				if !strings.Contains(all, want) {
					t.Errorf("evidence lacks %q:\n%s", want, all)
				}
			}
		})
	}
}

// The peer's policy (9, 9999) must never count, whichever side self is on.
func TestClassifyUsesOnlyOwnPolicies(t *testing.T) {
	chs := []graph.Channel{
		{ID: 1, Node1: peer, Node2: self, Node1Policy: pol(144, 200), Node2Policy: pol(80, 1)},
		{ID: 2, Node1: self, Node2: peer, Node1Policy: pol(80, 1), Node2Policy: pol(144, 200)},
	}
	got := Classify(graph.Node{PubKey: self}, slices.Values(chs))
	if got.Impl != LND || got.Layer != LayerPolicy {
		t.Fatalf("Classify() = %s by %s, want lnd by policy\nevidence: %q",
			got.Impl, got.Layer, got.Evidence)
	}
}
