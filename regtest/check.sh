#!/usr/bin/env bash
# Integration check for the lnd source (phase 1 exit criterion): on a regtest
# network, the number of nodes the observatory sees must equal
# `lncli describegraph | jq '.nodes | length'` after the snapshot, after a
# change that arrives through the update stream, and after a periodic
# snapshot replaces the graph.
#
# Usage: regtest/check.sh   (from the repository root; needs Docker and jq)
# Leaves nothing running: the network is removed on exit.
set -euo pipefail

cd "$(dirname "$0")"
work=$(mktemp -d)
compose=(docker compose -p observatory-regtest)
obs_pids=()

cleanup() {
	for p in "${obs_pids[@]}"; do kill "$p" 2>/dev/null || true; done
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT

btc() { "${compose[@]}" exec -T bitcoind bitcoin-cli -regtest -rpcuser=regtest -rpcpassword=regtest "$@"; }
lncli() { local n=$1; shift; "${compose[@]}" exec -T "$n" lncli --network regtest "$@"; }
mine() { btc generatetoaddress "$1" "$miner" >/dev/null; }

# wait_for <description> <command...>: retry for up to 120 s.
wait_for() {
	local what=$1; shift
	for _ in $(seq 120); do
		if "$@" >/dev/null 2>&1; then return 0; fi
		sleep 1
	done
	echo "timed out waiting for $what" >&2
	return 1
}

graph_counts() { lncli alice describegraph | jq -r '"\(.nodes | length) \(.edges | length)"'; }

echo "== starting bitcoind, alice and bob"
"${compose[@]}" up -d --quiet-pull
wait_for bitcoind btc getblockchaininfo
btc createwallet miner >/dev/null
miner=$(btc getnewaddress)
mine 101
wait_for "alice rpc" lncli alice getinfo
wait_for "bob rpc" lncli bob getinfo

echo "== funding alice and opening a public channel to bob"
btc sendtoaddress "$(lncli alice newaddress p2tr | jq -r .address)" 1 >/dev/null
mine 6
wait_for "alice funds" bash -c "$(declare -f lncli); compose=(${compose[*]}); \
	[[ \$(lncli alice walletbalance | jq -r .confirmed_balance) -gt 0 ]]"
bob_id=$(lncli bob getinfo | jq -r .identity_pubkey)
lncli alice connect "$bob_id@bob:9735" >/dev/null
lncli alice openchannel --node_key "$bob_id" --local_amt 1000000 >/dev/null
mine 6
wait_for "channel in alice's graph" bash -c "$(declare -f lncli graph_counts); \
	compose=(${compose[*]}); [[ \$(graph_counts) == '2 1' ]]"

"${compose[@]}" cp alice:/root/.lnd/tls.cert "$work/tls.cert"
"${compose[@]}" cp alice:/root/.lnd/data/chain/bitcoin/regtest/readonly.macaroon "$work/readonly.macaroon"
(cd .. && go build -o "$work/observatory" ./cmd/observatory)

# start_obs <log> <refresh interval>: run an observatory against alice.
start_obs() {
	"$work/observatory" --lnd.host localhost:10009 --lnd.tlscert "$work/tls.cert" \
		--lnd.macaroon "$work/readonly.macaroon" --log-interval 1s \
		--refresh-interval "$2" >"$1" 2>&1 &
	obs_pids+=($!)
}

# check <label> <log>: the latest counts in <log> must match lncli.
check() {
	local log=$2 want_nodes want_edges
	read -r want_nodes want_edges < <(graph_counts)
	wait_for "observatory to log $want_nodes nodes, $want_edges channels" bash -c \
		"[[ \$(grep -o 'nodes=[0-9]*' '$log' | tail -1) == nodes=$want_nodes && \
		   \$(grep -o 'channels=[0-9]*' '$log' | tail -1) == channels=$want_edges ]]" || {
		cat "$log" >&2
		return 1
	}
	echo "ok: $1: lncli describegraph has $want_nodes nodes and $want_edges channels;" \
		"observatory logged the same"
}

# The first observatory never refreshes during the run, so the second
# channel can only reach it through the stream.
echo "== starting the observatory against alice"
start_obs "$work/obs.log" 1h
check "snapshot" "$work/obs.log"

echo "== opening a second channel, seen only through the update stream"
lncli alice openchannel --node_key "$bob_id" --local_amt 500000 >/dev/null
mine 6
wait_for "second channel in alice's graph" bash -c "$(declare -f lncli graph_counts); \
	compose=(${compose[*]}); [[ \$(graph_counts) == '2 2' ]]"
check "stream" "$work/obs.log"
if grep -q "graph refreshed" "$work/obs.log"; then
	echo "the stream check observatory refreshed; the stream was not tested" >&2
	exit 1
fi

echo "== starting a second observatory that refreshes every 2 s"
start_obs "$work/refresh.log" 2s
wait_for "a graph refresh" grep -q "graph refreshed" "$work/refresh.log"
check "refresh" "$work/refresh.log"

echo "PASS"
