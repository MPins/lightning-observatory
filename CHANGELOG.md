# Changelog

Releases follow [Semantic Versioning](https://semver.org/) with Go module
tags (`v0.1.0`). Versions stay below `v1` while the metrics may still
change; `v0.1.0` closes phase 1 of the [roadmap](docs/roadmap.md).

## Unreleased

### Added

- Go module `github.com/MPins/lightning-observatory` with the
  `cmd/observatory` command.
- `observatory --version` prints the version and commit embedded by the Go
  toolchain.
- `internal/graph`: in-memory model of the public graph (nodes, channels,
  policies), the `Source` interface with snapshot and update stream, and a
  fake source in `graphtest` for tests.
- `internal/classify`: classifies a node as lnd, CLN, Eclair, LDK or
  unknown from its feature bits, then its own channel policies, then its
  color and alias, and reports the deciding layer and all evidence.
