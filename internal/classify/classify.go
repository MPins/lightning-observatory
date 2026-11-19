// Package classify guesses which Lightning implementation runs a node from
// what the node announces in gossip. The defaults it matches are surveyed in
// docs/fingerprints.md.
package classify

import (
	"encoding/hex"
	"fmt"
	"iter"

	"github.com/MPins/lightning-observatory/internal/graph"
)

// Implementation names a Lightning implementation.
type Implementation string

const (
	LND     Implementation = "lnd"
	CLN     Implementation = "cln"
	Eclair  Implementation = "eclair"
	LDK     Implementation = "ldk"
	Unknown Implementation = "unknown"
)

// Layer is the kind of signal that decided a classification. Layers are
// tried from the hardest for an operator to change to the easiest.
type Layer string

const (
	// LayerFeatures: feature bits, which come from the code.
	LayerFeatures Layer = "features"
	// LayerPolicy: cltv_expiry_delta and proportional fee of the node's
	// own channel_updates, which operators often keep.
	LayerPolicy Layer = "policy"
	// LayerAnnouncement: default color or alias.
	LayerAnnouncement Layer = "announcement"
	// LayerNone: nothing matched.
	LayerNone Layer = "none"
)

// Result is a classification with the signals behind it.
type Result struct {
	Impl  Implementation
	Layer Layer
	// Conflict is true when another layer points to a different
	// implementation. A rising share of conflicts means the defaults have
	// changed and docs/fingerprints.md needs a new survey.
	Conflict bool
	// Evidence lists every signal found, in layer order, including signals
	// that point to another implementation.
	Evidence []string
}

// Classify classifies node n from its announcement and the channels it is
// on. Only the policies n itself announced are used.
func Classify(n graph.Node, channels iter.Seq[graph.Channel]) Result {
	var r Result
	byFeatures := r.features(n.Features)
	byPolicy := r.policy(n.PubKey, channels)
	byAnnouncement := r.announcement(n)

	switch {
	case byFeatures != Unknown:
		r.Impl, r.Layer = byFeatures, LayerFeatures
	case byPolicy != Unknown:
		r.Impl, r.Layer = byPolicy, LayerPolicy
	case byAnnouncement != Unknown:
		r.Impl, r.Layer = byAnnouncement, LayerAnnouncement
	default:
		r.Impl, r.Layer = Unknown, LayerNone
	}
	for _, impl := range []Implementation{byFeatures, byPolicy, byAnnouncement} {
		if impl != Unknown && impl != r.Impl {
			r.Conflict = true
		}
	}
	return r
}

func (r *Result) note(format string, args ...any) {
	r.Evidence = append(r.Evidence, fmt.Sprintf(format, args...))
}

// features matches the default feature sets (docs/fingerprints.md,
// "Default sets" and "Bits that only one implementation sets").
func (r *Result) features(f graph.Features) Implementation {
	for _, bit := range []uint32{31, 261, 2023} {
		if f.Has(bit) {
			r.note("bit %d: only lnd announces it", bit)
			return LND
		}
	}
	// 81 (simple_taproot) can also be enabled on lnd, but such an lnd node
	// still announces the bits above.
	for _, bit := range []uint32{37, 81} {
		if f.Has(bit) {
			r.note("bit %d: only Eclair announces it by default", bit)
			return Eclair
		}
	}
	if f.Has(11) {
		r.note("bit 11 without lnd or Eclair bits: CLN")
		return CLN
	}
	if f.Has(43) && f.Has(47) && f.Has(63) {
		r.note("bits 43, 47, 63 without 11: LDK")
		if f.Has(23) {
			r.note("bit 23 with the LDK set: ldk-node default")
		}
		return LDK
	}
	return Unknown
}

// defaultPolicies are the (cltv_expiry_delta, ppm) defaults.
var defaultPolicies = []struct {
	delta uint16
	ppm   uint32
	impl  Implementation
	note  string
}{
	{80, 1, LND, "lnd default"},
	{34, 10, CLN, "CLN mainnet default"},
	{6, 10, CLN, "CLN test-network default"},
	{144, 200, Eclair, "Eclair default"},
	{72, 0, LDK, "LDK default"},
}

// policy picks the implementation whose defaults most of the node's own
// policies match; CLN's two profiles count together. A tie decides nothing.
func (r *Result) policy(pk graph.PubKey, channels iter.Seq[graph.Channel]) Implementation {
	counts := make([]int, len(defaultPolicies))
	byImpl := make(map[Implementation]int)
	total := 0
	for ch := range channels {
		p := ch.Node1Policy
		if ch.Node2 == pk {
			p = ch.Node2Policy
		}
		if p == nil {
			continue
		}
		total++
		for i, d := range defaultPolicies {
			if p.TimeLockDelta == d.delta && p.FeeRatePPM == d.ppm {
				counts[i]++
				byImpl[d.impl]++
			}
		}
	}

	for i, c := range counts {
		if c > 0 {
			r.note("policy (%d, %d ppm) on %d of %d channels: %s",
				defaultPolicies[i].delta, defaultPolicies[i].ppm, c, total,
				defaultPolicies[i].note)
		}
	}

	best, bestCount, tied := Unknown, 0, false
	for impl, c := range byImpl {
		switch {
		case c > bestCount:
			best, bestCount, tied = impl, c, false
		case c == bestCount:
			tied = true
		}
	}
	if tied {
		return Unknown
	}
	return best
}

// announcement matches default colors and aliases.
func (r *Result) announcement(n graph.Node) Implementation {
	impl := Unknown
	match := func(i Implementation, format string, args ...any) {
		r.note(format, args...)
		if impl == Unknown {
			impl = i
		}
	}

	switch n.Color {
	case [3]byte{0x33, 0x99, 0xff}:
		match(LND, "color #3399ff: lnd default")
	case [3]byte{0x49, 0xda, 0xaa}:
		match(Eclair, "color #49daaa: Eclair default")
	case [3]byte{n.PubKey[0], n.PubKey[1], n.PubKey[2]}:
		match(CLN, "color is the first 3 bytes of the node id: CLN default")
	case [3]byte{}:
		// Many operators pick black, so this only adds to the evidence.
		r.note("color #000000: ldk-node default, but common")
	}

	switch n.Alias {
	case hex.EncodeToString(n.PubKey[:10]):
		match(LND, "alias is the hex of the first 10 bytes of the node id: lnd default")
	case "eclair":
		match(Eclair, `alias "eclair": Eclair default`)
	}
	return impl
}
