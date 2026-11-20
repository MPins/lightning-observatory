#!/usr/bin/env bash
# Integration check for the lnd source (phase 1 exit criterion): on a regtest
# network, the number of nodes the observatory sees must equal
# `lncli describegraph | jq '.nodes | length'`, both after the snapshot and
# after a change that arrives through the update stream.
#
# Usage: regtest/check.sh   (from the repository root; needs Docker and jq)
# Leaves nothing running: the network is removed on exit.
set -euo pipefail

cd "$(dirname "$0")"
work=$(mktemp -d)
compose=(docker compose -p observatory-regtest)
obs_pid=

cleanup() {
	[[ -n $obs_pid ]] && kill "$obs_pid" 2>/dev/null || true
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

echo "== starting the observatory against alice"
"${compose[@]}" cp alice:/root/.lnd/tls.cert "$work/tls.cert"
"${compose[@]}" cp alice:/root/.lnd/data/chain/bitcoin/regtest/readonly.macaroon "$work/readonly.macaroon"
(cd .. && go build -o "$work/observatory" ./cmd/observatory)
"$work/observatory" --lnd.host localhost:10009 --lnd.tlscert "$work/tls.cert" \
	--lnd.macaroon "$work/readonly.macaroon" --log-interval 1s >"$work/obs.log" 2>&1 &
obs_pid=$!

# last_count <field>: the latest value the observatory logged.
last_count() { grep -o "$1=[0-9]*" "$work/obs.log" | tail -1 | cut -d= -f2; }

check() {
	local want_nodes want_edges
	read -r want_nodes want_edges < <(graph_counts)
	wait_for "observatory to log $want_nodes nodes, $want_edges channels" bash -c \
		"[[ \$(grep -o 'nodes=[0-9]*' '$work/obs.log' | tail -1) == nodes=$want_nodes && \
		   \$(grep -o 'channels=[0-9]*' '$work/obs.log' | tail -1) == channels=$want_edges ]]" || {
		cat "$work/obs.log" >&2
		return 1
	}
	echo "ok: $1: lncli describegraph has $want_nodes nodes and $want_edges channels;" \
		"observatory logged nodes=$(last_count nodes) channels=$(last_count channels)"
}

check "snapshot"

echo "== opening a second channel, seen only through the update stream"
lncli alice openchannel --node_key "$bob_id" --local_amt 500000 >/dev/null
mine 6
wait_for "second channel in alice's graph" bash -c "$(declare -f lncli graph_counts); \
	compose=(${compose[*]}); [[ \$(graph_counts) == '2 2' ]]"
check "stream"

echo "PASS"
