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
- Whether to depend on lnd's `brontide` and `lnwire` packages or write
  them is still open.

## Other ideas

- **Reachability.** Share of nodes announcing only Tor, only clearnet, or
  both.
- **Fee distribution.** Median and percentiles of the proportional and base
  fees over time.

## Out of scope

- Detecting the version of an implementation. Upgrade waves (phase 2) are
  observed feature changes, not versions.
- A history database. History lives in Prometheus.
- Alerting or a UI inside the service.
