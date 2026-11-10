# Implementation fingerprints

Default values each Lightning implementation exposes in gossip
(`node_announcement` and `channel_update`, BOLT 7), taken from the source of
the latest release of each one. Operators can override almost all of them;
these are what an unconfigured node announces.

Surveyed on 2026-11-09 from shallow clones of the release tags:

| Implementation | Tag | Commit |
|----------------|-----|--------|
| lnd | `v0.21.6-beta` | `ac3a47f` |
| Core Lightning | `v26.06.10` | `e8b1609e` |
| Eclair | `v0.14.6` | `50e046e` |
| LDK | `v0.2.7` | `5ce2ecb` |
| ldk-node (app on LDK 0.2) | `v0.7.1` | `edc8916` |

Sources are `path:line` inside each repository at that commit. Out of scope:
detecting the version of an implementation.

## Summary

### Feature bits in `node_announcement`

`R` = required (even bit), `o` = optional (odd bit), `–` = not announced by
default, `cfg` = only when a non-default option is enabled.

| Bit | Feature | lnd | CLN | Eclair | LDK |
|-----|---------|-----|-----|--------|-----|
| 0/1 | data_loss_protect | R | R | R | R |
| 4/5 | upfront_shutdown_script | o | o | – | o |
| 6/7 | gossip_queries | o | o | o | o¹ |
| 8/9 | var_onion_optin | R | R | R | R |
| 10/11 | gossip_queries_ex | – | o | o | – |
| 12/13 | static_remotekey | R | R | R | R |
| 14/15 | payment_secret | R | R | R | R |
| 16/17 | basic_mpp | o | o | o | o |
| 18/19 | large_channels (wumbo) | cfg | o | o | o |
| 22/23 | anchors_zero_fee_htlc_tx | o | o | o | cfg |
| 24/25 | route_blinding | o | o | o | o |
| 26/27 | shutdown_anysegwit | o | o | o | o |
| 28/29 | dual_fund | – | cfg | o | cfg |
| 30/31 | amp | o | – | – | – |
| 34/35 | quiesce | o | o | o | o |
| 36/37 | attribution_data | – | – | o | – |
| 38/39 | onion_messages | o | o | o | o¹ |
| 42/43 | provide_storage | – | o | cfg | o |
| 44/45 | channel_type | R | R | R | R |
| 46/47 | scid_alias | cfg | o | o | o |
| 50/51 | zeroconf | cfg | o | cfg | o |
| 54/55 | keysend | cfg | o² | cfg | o |
| 60/61 | simple_close | cfg | – | o | cfg |
| 62/63 | splice | – | o | o | o |
| 80/81 | simple_taproot | cfg | – | o | – |
| 260/261 | experimental accountability | o | – | – | – |
| 2022/2023 | script_enforced_lease | o | – | – | – |

¹ LDK only announces it if the app wires in `P2PGossipSync` /
`OnionMessenger`.
² From CLN's built-in `keysend` plugin; it disappears if the plugin is disabled.

Default sets, as announced:

- **lnd**: {0, 5, 7, 8, 12, 14, 17, 23, 25, 27, 31, 35, 39, 44, 261, 2023}
- **CLN**: {0, 5, 7, 8, 11, 12, 14, 17, 19, 23, 25, 27, 35, 39, 43, 44, 47, 51, 55, 63}
- **Eclair**: {0, 7, 8, 11, 12, 14, 17, 19, 23, 25, 27, 29, 35, 37, 39, 44, 47, 61, 63, 81}
- **LDK**: {0, 5, 7, 8, 12, 14, 17, 19, 25, 27, 35, 39, 43, 44, 47, 51, 55, 63}
- **ldk-node**: the LDK set plus 23 (anchors are on by default), that is
  the CLN set without 11. Feature bits alone separate ldk-node from CLN only
  by bit 11; the channel policy and the color separate them clearly.

Bits that only one implementation sets:

- lnd: 31, 261, 2023.
- Eclair: 37, 81 by default; the custom bits 148/149 and 564/565, which are off
  by default.
- LDK: 141, 153 and 259, all off by default. ldk-node adds 729 (LSPS) when
  it advertises an LSP service, off by default.
- CLN: none by default. CLN differs from LDK by having 11 and 23 and lacking
  nothing LDK has.

### `channel_update` and `node_announcement`

| Signal | lnd | CLN (mainnet) | Eclair | LDK |
|--------|-----|---------------|--------|-----|
| `cltv_expiry_delta` | 80 | 34 (6 on testnet/regtest/signet) | 144 | 72 (floor 48) |
| `fee_base_msat` | 1000 | 1000 (1 on test networks) | 1000 | 1000 |
| `fee_proportional_millionths` | 1 | 10 | 200 | 0 |
| `htlc_minimum_msat` | max(peer's, 1000) | peer's | peer's | peer's |
| `htlc_maximum_msat` | min(peer's max in-flight, capacity): 99% of capacity with an lnd peer | capacity − peer's reserve, capped by peer's max in-flight | steps down with local balance; at most min(50 mBTC, 45% of capacity) | min(90% of capacity, peer's max in-flight) |
| Color | `#3399ff` | first 3 bytes of the node id | `#49daaa` | none (set by the app); always `#000000` in ldk-node |
| Alias | hex of the first 10 bytes of the node id | ADJECTIVE+NOUN derived from the node id, e.g. `SLICKERPHOTO` | `eclair` | none (set by the app); ldk-node requires one to announce |
| `channel_update` refresh | every 24 h | every 13 days | every 10 days | set by the app |

The (`cltv_expiry_delta`, `fee_proportional_millionths`) pair alone separates
all four defaults: (80, 1), (34, 10), (144, 200), (72, 0).

## lnd v0.21.6-beta

Release builds; `ProtocolOptions` fields left unset are `false`
(`config.go:837-841`). The config-to-bits wiring is in `server.go:739-757`.

| Signal | Default | Source |
|--------|---------|--------|
| Node-announcement feature set | see summary | `feature/default_sets.go:13-122` |
| Bit 261 experimental accountability | on, node announcement only | `feature/default_sets.go:111`, `lnwire/features.go:300` |
| Bit 2023 script_enforced_lease | on | `feature/default_sets.go:74`, `lnwire/features.go:268` |
| Bit 31 amp | on | `feature/default_sets.go:56` |
| Bits 19, 47, 51 | off: `protocol.wumbo-channels`, `option-scid-alias`, `zero-conf` | `lncfg/protocol.go:28,54,58` |
| Bit 55 keysend | off: `accept-keysend` | `config.go:532`, `server.go:745` |
| Bits 61/161, 81/181, 2025 | off: `rbf-coop-close`, `simple-taproot-chans`, `simple-taproot-overlay-chans` | `lncfg/protocol.go:32-40`, `lnwire/features.go:242-314` |
| Custom bits | `protocol.custom-nodeann` | `lncfg/protocol.go:129` |
| `cltv_expiry_delta` | 80, the same on every network | `chainreg/chainregistry.go:129` |
| `htlc_minimum_msat` | max(peer's, 1000) | `chainreg/chainregistry.go:119`, `funding/manager.go:3680-3686` |
| `fee_base_msat` / ppm | 1000 / 1 | `chainreg/chainregistry.go:122,125` |
| `htlc_maximum_msat` | min(peer's max in-flight, capacity); lnd asks for capacity − 1% reserve | `funding/manager.go:3691-3695`, `server.go:1722-1727` |
| `message_flags` | 1 | `funding/manager.go:4642` |
| Color | `#3399FF`; a color already stored in the DB wins | `config.go:110`, `server.go:5805-5809` |
| Alias | hex of the first 10 bytes of the pubkey, on first start only | `server.go:5828-5833` |
| Addresses | none unless `externalip`, NAT/UPnP or Tor | `server.go:5741-5778` |
| Refresh | re-signs updates older than 24 h, checked every 30 min | `server.go:1190-1191`, `discovery/gossiper.go:1889,1940` |

## Core Lightning v26.06.10

The `testnet` profile (cltv 6, base fee 1) applies to regtest, signet, testnet
and testnet4 (`bitcoin/chainparams.c:57-185`, `lightningd/options.c:1863-1866`).

| Signal | Default | Source |
|--------|---------|--------|
| Node-announcement feature set | see summary | `lightningd/lightningd.c:913-935`, filter in `common/features.c:35-146` |
| Bit 55 keysend | on, from the built-in plugin | `plugins/keysend.c:17,590` |
| Bit 63 splice | on; `--experimental-splicing` is a no-op | `lightningd/options.c:1262-1273` |
| Bit 29 dual_fund | off: `--experimental-dual-fund` | `lightningd/options.c:1250-1259` |
| Bit 105 shutdown_wrong_funding | off: `--experimental-shutdown-wrong-funding` | `lightningd/options.c:1276-1281` |
| Bit 49 payment_metadata | never in the node announcement | `common/features.c:129` |
| `cltv_expiry_delta` | 34 (mainnet), 6 (test networks) | `lightningd/options.c:1075`, `:1002` |
| `fee_base_msat` / ppm | 1000 / 10 (mainnet); 1 / 10 (test networks) | `lightningd/options.c:1087-1089`, `:1009-1011` |
| `htlc_minimum_msat` | max(0, peer's) | `lightningd/channel.c:703-707` |
| `htlc_maximum_msat` | min(capacity − peer's reserve, peer's max in-flight) | `lightningd/channel.c:708-712` |
| `message_flags` | 1 (+2 for `dont_forward`) | `lightningd/gossip_generation.c:117-119` |
| Color | first 3 bytes of the compressed node id, so it starts with `02` or `03` | `lightningd/options.c:1803-1805` |
| Alias | adjective + noun, indexed by `k[3..10]` and `k[11..18]` of the node id | `lightningd/options.c:1785-1833` |
| Addresses | configured ones, else the IP discovered through `remote_addr` | `lightningd/gossip_generation.c:377-408` |
| `option_will_fund` TLV | only after `setleaserates` | `lightningd/gossip_control.c:392-394` |
| Refresh | every 13 days | `lightningd/channel_gossip.c:189,608-609` |

## Eclair v0.14.6

Paths are relative to `eclair-core/src/main/`. Only `NodeFeature`s go into the
node announcement (`scala/fr/acinq/eclair/Features.scala:177`).

| Signal | Default | Source |
|--------|---------|--------|
| Node-announcement feature set | see summary | `resources/reference.conf:60-102` |
| Bit 44 channel_type | required | `resources/reference.conf:91` |
| Bit 48/49 payment_metadata | on, but invoice only | `scala/fr/acinq/eclair/Features.scala:382` |
| Custom bits 128-564 | all off; 148/149 and 564/565 would be announced | `scala/fr/acinq/eclair/Features.scala:417-470` |
| `cltv_expiry_delta` | 144, the same on every network | `resources/reference.conf:159` |
| `fee_base_msat` / ppm | 1000 / 200 for public channels | `resources/reference.conf:248-249` |
| `htlc_minimum_msat` | the peer's | `scala/fr/acinq/eclair/channel/Helpers.scala:361` |
| `htlc_maximum_msat` | steps by local balance: ≤ 1k / 10k / 100k / 200k / 400k / 800k / 1.6M sat → 0 / 1k / 10k / 100k / 200k / 400k / 800k sat | `resources/reference.conf:201-227`, `scala/fr/acinq/eclair/channel/Helpers.scala:389-403` |
| `message_flags` | 1 (+2 for private channels) | `scala/fr/acinq/eclair/router/Announcements.scala:124` |
| Color | `49daaa` | `resources/reference.conf:50` |
| Alias | `eclair` | `resources/reference.conf:49` |
| Addresses | none (`public-ips = []`) | `resources/reference.conf:7` |
| `option_will_fund` TLV | absent (`funding-rates = []`) | `resources/reference.conf:385` |
| Refresh | every 10 days | `scala/fr/acinq/eclair/channel/fsm/Channel.scala:189` |

## LDK v0.2.7

LDK is a library: these are the defaults of `UserConfig` / `ChannelConfig`,
and apps such as ldk-node may change them. Alias, color and addresses always
come from the app (`lightning/src/ln/peer_handler.rs:3651-3671`). The
node-announcement features are the union of every handler's
`provided_node_features()` (`lightning/src/ln/peer_handler.rs:3662-3666`).

| Signal | Default | Source |
|--------|---------|--------|
| Node-announcement feature set | see summary | `lightning/src/ln/channelmanager.rs:15744-15813` |
| Bit 23 anchors | off: `negotiate_anchors_zero_fee_htlc_tx` | `lightning/src/util/config.rs:257` |
| Bit 47 scid_alias | always announced | `lightning/src/ln/channelmanager.rs:15792` |
| Bits 141, 153 | experimental, off | `lightning-types/src/features.rs:711,722` |
| Bit 259 dns_resolver (bLIP-32) | only with the DNS resolver handler | `lightning-types/src/features.rs:744` |
| Never announced | 3, 21, 31, 49 | `lightning-types/src/features.rs:181-211` |
| `cltv_expiry_delta` | 72, floor 48 | `lightning/src/util/config.rs:665`, `lightning/src/ln/channelmanager.rs:3084` |
| `fee_base_msat` / ppm | 1000 / 0 | `lightning/src/util/config.rs:663-664` |
| `htlc_minimum_msat` | the peer's | `lightning/src/ln/channel.rs:4302` |
| `htlc_maximum_msat` | min(90% of capacity, peer's max in-flight); LDK limits inbound in-flight to 10%, so its peers tend to announce 10% | `lightning/src/ln/channel.rs:4300,13114-13121`, `lightning/src/util/config.rs:252` |
| `message_flags` | 1 | `lightning/src/ln/channelmanager.rs:5195` |
| `channel_update` timestamp | follows block header time, not wall clock | `lightning/src/ln/channel.rs:11405-11406` |

## ldk-node v0.7.1

The most common app built on LDK; it depends on `lightning` 0.2
(`Cargo.toml:32`). It keeps LDK's channel policy and changes the node
announcement.

| Signal | Default | Source |
|--------|---------|--------|
| `cltv_expiry_delta`, fees | LDK's: 72, 1000 / 0 | `src/config.rs:494-497` |
| `default_cltv_expiry_delta` = 144 | in the docs table only; no such field exists | `src/config.rs:114` |
| Bit 23 anchors | on (`anchor_channels_config: Some`) | `src/config.rs:198,326-327` |
| Bit 7 gossip_queries | on with the default P2P gossip; off with Rapid Gossip Sync | `src/builder.rs:1599,1685-1696` |
| Bit 39 onion_messages | on, `OnionMessenger` is always built | `src/builder.rs:1571-1583` |
| Bit 729 LSPS | only when an LSP service is advertised | `src/message_handler.rs:84-90`; LDK `lightning-liquidity/src/manager.rs:66,925-929` |
| Color | always `#000000` (`[0; 3]`) | `src/lib.rs:501` |
| Alias | none by default; without alias and listening addresses the node does not announce channels | `src/config.rs:305-315,331-333` |
| `node_announcement` refresh | every hour | `src/config.rs:70` |
| LSPS2 service | max inbound HTLC in flight 100% of the channel | `src/builder.rs:1478` |

## Changes from the 2023/2024 templates

The Python classifier in `legacy/src/` used these values:

- lnd `cltv_expiry_delta` 40 or 80: now only 80.
- CLN `cltv_expiry_delta` 6: that is the test-network value; mainnet is 34.
- Eclair 144 and `#49daaa`, lnd `#3399ff`, LDK 72: unchanged.
- It did not use the fee rate, `htlc_minimum_msat`, `htlc_maximum_msat`, CLN's
  color rule or the aliases.
