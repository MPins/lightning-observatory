# Lightning Observatory

An observatory for the public Lightning Network graph. It watches the
graph continuously and exports metrics to Prometheus, with Grafana
dashboards and alert rules on top:

- **Implementation mix.** Which implementation each node runs (LND, Core
  Lightning, Eclair, LDK), by node count and by capacity.
- **Feature adoption.** How many nodes, and how much capacity, announce
  each feature bit.
- **Liveness.** How fresh the channel updates are and how many channels are
  disabled, as a measure of how many nodes are active.
- **Shape of the network.** Concentration of capacity, use of default
  configurations, and sudden changes in any of these.

The implementation is inferred from the defaults each project ships for
public information. Examples are feature bits in the `node_announcement`,
the routing policy in the `channel_update`, and generated aliases and
colors. Knowing the implementation mix helps when deciding on protocol
upgrades and feature deprecations. It also shows how exposed the network
is to a bug in a single implementation.

This project started at the Chaincode Hackathon as **Lightning Nodenamer**
([proposal](legacy/Hackathon-Project-NodeNamer.pdf)).

## Status

**Being rewritten in Go.** The first version was a Python script that read
an `lncli describegraph` dump and wrote logs for Promtail and Loki. It now
lives in [`legacy/`](legacy/) for reference. The rewrite starts from
scratch, and there is no runnable Go code yet.

## Design

A long-running Go service keeps the graph in memory and derives metrics
from it on `/metrics`. The classifier and the metrics read the graph
through a source interface and do not know where it came from:

1. **lnd over gRPC** (first): `DescribeGraph` at startup, then
   `SubscribeChannelGraph`. Needs only a `readonly` macaroon.
2. **Gossip** (later): connect to the network as a peer (BOLT 7).

Alerts are Prometheus rules kept in this repository; the service has no
alerting or UI of its own. Detecting the *version* of an implementation is
out of scope, because there is no reliable signal for it.

## Fingerprints

[`docs/fingerprints.md`](docs/fingerprints.md) lists the default gossip
values of the latest release of each implementation (lnd v0.21.6-beta,
Core Lightning v26.06.10, Eclair v0.14.6, LDK v0.2.7 and the ldk-node
v0.7.1 app), with the `path:line` in each code base where every value comes
from. The classifier will be built from that table.

## Roadmap

[`docs/roadmap.md`](docs/roadmap.md) describes the three phases and every
planned metric. In short:

1. **lnd gRPC source.** Implementation mix, feature adoption, update
   freshness, disabled channels, default configurations, concentration,
   alert rules and the Grafana dashboard.
2. **Comparing with the previous graph.** Churn of nodes and channels, and
   upgrade waves seen as changes in feature bits.
3. **Gossip source.** Raw update rate and propagation delay.

Done so far:

- [x] Move the Python version to `legacy/`.
- [x] Survey the current default fingerprints ([`docs/fingerprints.md`](docs/fingerprints.md)).

Integration tests will run on a regtest network in Docker.

## Legacy version

See [`legacy/README.md`](legacy/README.md) for how the Python script worked
and how to run it. The old Grafana dashboard export is
[`lightning-nodenamer-grafana-dashboard.json`](lightning-nodenamer-grafana-dashboard.json).
It will be replaced when the dashboard is rebuilt on the Prometheus
metrics.
