#!/usr/bin/env bash
# runner-egress-firewall.sh — Egress Layer 1 for agentic-delegator runners.
#
# Creates the dedicated runner bridge network and applies DOCKER-USER rules that
# black-hole (DROP) runner egress to private / link-local / cloud-metadata
# ranges while leaving the public internet reachable. Idempotent; run as root.
#
#   runner-egress-firewall.sh up     # create network + apply rules (default)
#   runner-egress-firewall.sh down   # remove rules (network left intact)
set -euo pipefail

NET=runner-net
BR=br-runner
SUBNET=172.31.255.0/24
GATEWAY=172.31.255.1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RULES_FILE="${EGRESS_RULES_FILE:-$SCRIPT_DIR/egress-filter.rules}"

# Default DROP set; overridden by the sourced rules file when present.
DROP_CIDRS=(10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 169.254.0.0/16)
if [ -r "$RULES_FILE" ]; then
  # shellcheck source=/dev/null
  source "$RULES_FILE"
fi

require_root() {
  [ "$(id -u)" -eq 0 ] || { echo "must run as root" >&2; exit 1; }
}

# The rules live in DOCKER-USER, which exists only under Docker's iptables
# firewall backend. Docker 29's experimental nftables backend has no such chain
# and these rules would silently no-op — refuse rather than fail open.
assert_docker_user_chain() {
  iptables -L -n DOCKER-USER >/dev/null 2>&1 || {
    echo "DOCKER-USER chain not found — Docker must use the iptables firewall backend." >&2
    echo "(Docker 29 nftables backend has no DOCKER-USER chain; these rules would no-op.)" >&2
    exit 1
  }
}

ensure_network() {
  if ! docker network inspect "$NET" >/dev/null 2>&1; then
    echo "[firewall] creating docker network $NET ($SUBNET, bridge $BR)"
    docker network create \
      --driver bridge \
      --subnet "$SUBNET" \
      --gateway "$GATEWAY" \
      --opt com.docker.network.bridge.name="$BR" \
      --opt com.docker.network.bridge.enable_ip_masquerade=true \
      "$NET" >/dev/null
  fi
}

# add_rule appends a DOCKER-USER rule iff not already present (idempotent).
add_rule() {
  iptables -C DOCKER-USER "$@" 2>/dev/null || iptables -A DOCKER-USER "$@"
}

# del_rule removes every copy of a DOCKER-USER rule (idempotent).
del_rule() {
  while iptables -C DOCKER-USER "$@" 2>/dev/null; do
    iptables -D DOCKER-USER "$@"
  done
}

up() {
  require_root
  ensure_network
  assert_docker_user_chain

  # 0) Allow established/related return traffic for the runner subnet, inserted
  #    at the top. Currently redundant with the destination-scoped DROPs below
  #    (return traffic is -s public -d subnet and matches no DROP), but becomes
  #    load-bearing if a broader default-deny is ever added (Layer 2).
  # SC2054: the comma in ESTABLISHED,RELATED is iptables conntrack syntax, not
  # an array separator — the array elements are space-separated as required.
  # shellcheck disable=SC2054
  local ret=(-s "$SUBNET" -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN)
  iptables -C DOCKER-USER "${ret[@]}" 2>/dev/null || iptables -I DOCKER-USER 1 "${ret[@]}"

  # 1) DROP egress from the runner subnet to private/link-local/metadata ranges.
  local cidr
  for cidr in "${DROP_CIDRS[@]}"; do
    add_rule -s "$SUBNET" -d "$cidr" -j DROP
  done
  echo "[firewall] applied DOCKER-USER egress DROP rules for $SUBNET"
}

down() {
  require_root
  local cidr
  for cidr in "${DROP_CIDRS[@]}"; do
    del_rule -s "$SUBNET" -d "$cidr" -j DROP
  done
  del_rule -s "$SUBNET" -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
  echo "[firewall] removed DOCKER-USER egress rules for $SUBNET"
  echo "[firewall] network $NET left intact ('docker network rm $NET' to remove)"
}

case "${1:-up}" in
  up)   up ;;
  down) down ;;
  *)    echo "usage: $0 [up|down]" >&2; exit 2 ;;
esac
