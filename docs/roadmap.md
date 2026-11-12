# Roadmap

Lightning Observatory keeps a model of the public Lightning graph in memory
and derives metrics from it for Prometheus. Alerts are Prometheus rules and
dashboards are Grafana; the service itself only exports `/metrics`.

The classifier and the metrics never know where the graph came from. They
read it through a source interface, so the lnd gRPC source of phase 1 and
the gossip source of phase 3 are interchangeable.

## Design rules

- **No pubkey labels.** With more than 10,000 nodes, a per-node label would
  create one series per node. Metrics aggregate by implementation, feature
  bit, age bucket and similar low-cardinality dimensions.
- **Ratios are PromQL.** The service exports counts; shares and
  proportions are computed at query time.
- **Activity depends on the implementation.** Default refresh intervals
  differ: lnd re-signs its `channel_update`s every 24 h, Eclair every 10
  days, CLN every 13 days ([fingerprints](fingerprints.md)). An update age
  that is normal for Eclair is a warning sign for lnd.
- **Read-only access.** The lnd source only needs a `readonly` macaroon.

## Phase 1: lnd gRPC source

The graph comes from an lnd node: `DescribeGraph` at startup, then
`SubscribeChannelGraph`. lnd has already checked each channel against the
chain and dropped zombie channels, and it provides the capacity.

- **Implementation mix.** Node count and capacity per implementation
  (LND, CLN, Eclair, LDK, unknown), with the layer that classified each
  node. Read as risk: the capacity exposed to a bug in one implementation.
- **Feature adoption.** Nodes and capacity announcing each feature bit, per
  implementation. The capacity-weighted view shows whether large nodes adopt
  a feature first.
- **Update freshness.** Histogram of the age of each channel's last
  `channel_update`, per implementation, with buckets at 1 h, 1 d, 7 d and
  14 d (the BOLT 7 pruning horizon). A node counts as active when its updates
  are younger than its implementation's refresh interval.
- **Disabled channels.** Share of channels whose latest `channel_update`
  has the `disabled` flag. lnd disables a channel 20 minutes after losing the
  peer, so this tracks nodes that are offline.
- **Default configuration.** Nodes still announcing their implementation's
  default fees and `cltv_expiry_delta`.
- **Concentration.** Capacity share of the top 10, 50 and 100 nodes, and the
  number of nodes that together hold 50% of the capacity.
- **Alert rules.** Prometheus rules kept in the repository, comparing the
  current window with the same window a week earlier (`offset 1w`): drop in
  active nodes, jump in disabled channels, large capacity change, unusual
  update rate. With the gRPC source the update rate is what lnd accepted,
  after its own filtering.
- **Grafana dashboard** rebuilt on these metrics.

## Phase 2: comparing with the previous graph

These metrics need the previous graph, also across restarts. The plan is to
keep only the last snapshot in a local file, with no history database; the
time series stay in Prometheus.

- **Churn.** Nodes that appear and disappear, channels opened and closed,
  per day.
- **Upgrade waves.** Nodes whose feature set changes, e.g. an lnd node that
  starts announcing bit 261 has upgraded. Counting these transitions shows
  how fast each release spreads, without detecting versions.

## Phase 3: gossip source

Connect to the network as a peer (BOLT 8 handshake, BOLT 7 gossip queries)
instead of reading lnd. Validating `channel_announcement`s and getting
capacities needs a chain backend (bitcoind, Esplora or an lnd/btcd node).
Several peers are needed for a complete graph.

- **Raw update rate**, including messages lnd would filter or rate-limit.
- **Propagation delay**: the update timestamp against the time it arrives.
- **View comparison**: what lnd keeps against what the network sends.
- **Several vantage points.** One collector with many peers already covers
  the graph; collectors in different places of the network are what make
  propagation measurable. Each hop batches gossip (lnd every 90 s, CLN and
  Eclair every 60 s), so an update reaches different points minutes apart,
  and the arrival-time difference between collectors is the propagation
  measure. NTP-level clocks are enough at that scale. Collectors only
  record raw messages with arrival time and a `collector` label; validation
  against the chain happens once, centrally. Start with one collector, but
  keep the record shape ready for more.
- **Community collectors (later).** A collector that small could be run by
  others. Gossip messages are signed by their authors, so a collector cannot
  forge content, only drop it or lie about arrival times; propagation
  metrics then need robust statistics (medians, per-collector consistency
  checks) and a way to exclude a collector. A collector uses its own fresh
  node key, never the operator's node key, macaroon or seed. It needs a
  central ingestion endpoint, which the single-collector setup does not.
  Each collector declares a coarse `region` label (continent or country)
  instead of having its IP geolocated.
- **Partitions.** Per-hop batching (60–90 s) dominates delay, so geography
  barely changes propagation time; it matters for cuts (a country's
  internet shutdown, a Tor outage). Signals: channels between two groups
  disabled at once, updates from one region stopping, and collectors that
  stop agreeing on which channels are fresh. Telling "a region went
  offline" from "the network split" needs collectors on both sides of the
  cut; from one side both look the same.
- Whether to depend on lnd's `brontide` and `lnwire` packages or write
  them is still open.

## Node roles

A second axis next to the implementation: what a node is in the world.
Exchange, custodial wallet (e.g. Wallet of Satoshi), swap service, Lightning
Service Provider (LSP), routing node, unknown. Roles are low-cardinality, so
they fit the no-pubkey rule: capacity per role shows how much of the network
sits with custodians. Like the implementation classifier, it returns the
role, the layer that decided it and the evidence.

What the public graph can and cannot show:

- **Zero-conf channels are invisible.** An LSP opens them to mobile wallets
  as unannounced channels, so they never reach the gossip or
  `DescribeGraph`.
- **Bit 51 (`option_zeroconf`) only means something per implementation.**
  CLN and LDK announce it by default; lnd and Eclair only when configured
  ([fingerprints](fingerprints.md)). An lnd or Eclair node announcing 51
  (and 47, `option_scid_alias`) turned zero-conf on deliberately: an LSP
  candidate. On CLN and LDK the bit says nothing.
- **Known entities.** A versioned list of pubkey → entity → role, curated by
  hand from public sources. The alias alone is self-declared and can be
  copied, so it is evidence, not proof.
- **Behaviour heuristics.** Channel count and capacity, fee levels, how often
  the node rewrites its fees (swap services and active routers adjust fees
  dynamically). Weak on their own; they need the known-entity list as ground
  truth before they are trusted.
- **Bit 729 (LSPS).** bLIP-52 lets an LSP announce feature bit 729 in its
  `node_announcement`. Only LDK does it today, and only when the service is
  configured with `advertise_service` (`lightning-liquidity`, also exposed
  by ldk-node). CLN's LSPS plugin has the bit commented out, and Eclair and
  lnd have no LSPS service. lnd keeps unknown bits and returns them in
  `DescribeGraph` and `SubscribeChannelGraph` with `is_known: false`, so the
  signal is already visible in phase 1. Its absence proves nothing.
- **Eclair LSPs (Phoenix-style).** ACINQ's LSP for Phoenix runs Eclair with
  its own protocol, not LSPS. Its funding features (`on_the_fly_funding`
  560/561, `funding_fee_credit` 562/563) are init-only and never reach the
  graph. Two node features that serve mobile wallets are off by default in
  Eclair (`reference.conf`): `option_provide_storage` (42/43) and
  `trampoline_payment_prototype` (148/149). An Eclair node announcing them
  is an LSP candidate. CLN and LDK announce 43 by default, so there it says
  nothing. The wallets themselves are unannounced, so the trampoline bit in
  the graph marks the node that routes for them. CLN and LDK use the BOLT
  proposal's bit 56/57 instead (off by default), which marks the same role.
- **Asking the node (needs phase 3).** The LSP side answers
  `lsps0.list_protocols` (BOLT 8 custom message type 37913) with the
  protocols it serves; clients only send the request. A peer connection can
  ask, which confirms an LSP that does not announce bit 729. Clients cannot
  be probed: they are usually unannounced mobile wallets. This is an active
  probe and needs its own decision.

## Other ideas

- **Reachability.** Share of nodes announcing only Tor, only clearnet, or
  both.
- **Node location.** Country and ASN of the clearnet addresses in
  `node_announcement`s, from an offline database (no per-IP lookups to an
  external service). Shows hosting concentration and gives partitions a
  "where". Tor-only nodes have no location.
- **Fee distribution.** Median and percentiles of the proportional and base
  fees over time.

## Out of scope

- Detecting the version of an implementation. Upgrade waves (phase 2) are
  observed feature changes, not versions.
- A history database. History lives in Prometheus.
- Alerting or a UI inside the service.
