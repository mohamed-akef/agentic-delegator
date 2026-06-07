#!/usr/bin/env bash
# install.sh — install/upgrade the agentic-delegator SaaS deployment on a host.
#
# Run as root from the untarred release directory (the dist/ produced by the
# release workflow). Idempotent: safe to re-run on upgrade. Installs the binary,
# the systemd units, and the runner egress firewall, then enables them.
#
# Shipping files in the tarball is not the same as installing them; this is the
# step that copies units into place, runs daemon-reload, and enables the units.
set -euo pipefail

[ "$(id -u)" -eq 0 ] || { echo "must run as root" >&2; exit 1; }

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[install] binary -> /usr/local/bin/agentic-delegator-saas"
install -m 0755 "$SRC_DIR/agentic-delegator" /usr/local/bin/agentic-delegator-saas

echo "[install] firewall script + rules -> /usr/local/sbin"
install -m 0755 "$SRC_DIR/firewall/runner-egress-firewall.sh" /usr/local/sbin/runner-egress-firewall.sh
install -m 0644 "$SRC_DIR/firewall/egress-filter.rules"        /usr/local/sbin/egress-filter.rules

echo "[install] systemd units -> /etc/systemd/system"
install -m 0644 "$SRC_DIR/firewall/runner-egress-firewall.service" /etc/systemd/system/runner-egress-firewall.service
install -m 0644 "$SRC_DIR/agentic-delegator-saas.service"          /etc/systemd/system/agentic-delegator-saas.service

# EnvironmentFile the SaaS unit reads. Root-only (0600): it carries secrets
# (AGENTIC_MASTER_KEY, GH app key). systemd reads it as root before dropping to
# User=agentic-delegator, so the service user need not read it.
mkdir -p /etc/agentic-delegator
if [ ! -f /etc/agentic-delegator/env ]; then
  echo "[install] creating /etc/agentic-delegator/env stub (edit before starting)"
  umask 077
  cat > /etc/agentic-delegator/env <<'EOF'
# agentic-delegator SaaS environment. See docs/saas-setup.md for the full list.
# Required: AGENTIC_MASTER_KEY, GitHub App/OAuth (AGENTIC_GH_*), DELEGATOR_DSN.
#
# Egress Layer 1 (production): attach runners to the firewalled bridge network.
AGENTIC_RUNNER_NETWORK=runner-net
# AGENTIC_RUNNER_DNS=1.1.1.1,1.0.0.1
#
# WorkDir/Logs MUST live outside the unit's PrivateTmp namespace (real host path
# dockerd can see). Do not point these at /tmp.
AGENTIC_WORK_DIR=/var/lib/agentic-delegator/work
# AGENTIC_LOG_DIR defaults to ${AGENTIC_WORK_DIR}/logs.
EOF
  chmod 0600 /etc/agentic-delegator/env
fi

# Create the system user the SaaS unit runs as, if absent.
if ! id -u agentic-delegator >/dev/null 2>&1; then
  echo "[install] creating system user agentic-delegator"
  useradd --system --no-create-home --shell /usr/sbin/nologin agentic-delegator
fi

systemctl daemon-reload
# Firewall first (the SaaS unit orders after it), then the service.
echo "[install] enabling runner-egress-firewall.service"
systemctl enable --now runner-egress-firewall.service
echo "[install] enabling agentic-delegator-saas.service"
systemctl enable agentic-delegator-saas.service

echo "[install] done."
echo "          1. edit /etc/agentic-delegator/env"
echo "          2. /usr/local/bin/agentic-delegator-saas migrate up"
echo "          3. systemctl start agentic-delegator-saas"
