# Lightning Nodenamer

The Lightning Network is implemented by several projects: LND, Core
Lightning, Eclair and LDK. Each one ships its own defaults for information
that every node makes public through gossip, and those defaults can be used
to tell which implementation a node runs. Examples:

- Feature bits in the `node_announcement`.
- Routing policy in the `channel_update`: `cltv_expiry_delta`, fees,
  `htlc_minimum_msat` and `htlc_maximum_msat`.
- Generated node aliases and RGB colors.

Knowing the implementation mix of the network helps when deciding on
protocol upgrades and feature deprecations. It also shows how exposed the
network is to a bug in a single implementation.

This project started at the Chaincode Hackathon
([proposal](legacy/Hackathon-Project-NodeNamer.pdf)).

## Status

**Being rewritten in Go.** The first version was a Python script that read
an `lncli describegraph` dump and wrote logs for Promtail and Loki. It now
lives in [`legacy/`](legacy/) for reference. The rewrite starts from
scratch, and there is no runnable Go code yet.

## What it will be

A long-running Go service that:

1. Reads the channel graph from an lnd node over gRPC: `DescribeGraph` at
   startup, then `SubscribeChannelGraph` for updates. Later it will also
   read gossip directly from the network as a peer (BOLT 7).
2. Classifies every node as LND, CLN, Eclair, LDK or unknown, and records
   which evidence was used (feature bits, channel policy, color, alias).
3. Exposes node counts and capacity per implementation on `/metrics` for
   Prometheus, with a Grafana dashboard on top.

Detecting the *version* of an implementation is out of scope. The work so
far found no reliable signal for it.

## Fingerprints

[`docs/fingerprints.md`](docs/fingerprints.md) lists the default gossip
values of the latest release of each implementation (lnd v0.21.6-beta,
Core Lightning v26.06.10, Eclair v0.14.6 and LDK v0.2.7), with the
`path:line` in each code base where every value comes from. The
classifier will be built from that table.

## Roadmap

- [x] Move the Python version to `legacy/`.
- [x] Survey the current default fingerprints ([`docs/fingerprints.md`](docs/fingerprints.md)).
- [ ] Go module skeleton (`cmd/nodenamer`).
- [ ] Pure classifier with table-driven tests for each implementation.
- [ ] lnd gRPC source, checked against `lncli describegraph` on regtest.
- [ ] Prometheus metrics (`nodenamer_nodes`, `nodenamer_capacity_sat`).
- [ ] Grafana dashboard rebuilt on the new metrics.
- [ ] Gossip source that connects as a peer.

Integration tests will run on a regtest network in Docker.

## Legacy version

See [`legacy/README.md`](legacy/README.md) for how the Python script worked
and how to run it. The old Grafana dashboard export is
[`lightning-nodenamer-grafana-dashboard.json`](lightning-nodenamer-grafana-dashboard.json).
It will be replaced when the dashboard is rebuilt on the Prometheus
metrics.
