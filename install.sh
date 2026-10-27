#!/usr/bin/env bash
# Installs or upgrades moxy as a systemd service on the Proxmox host.
#
#   curl -fsSLO https://raw.githubusercontent.com/njyeung/moxy/main/install.sh
#   sudo bash install.sh
#
# Pin a version with MOXY_VERSION=v1.2.3. Re-running upgrades the binary and
# restarts the service; an existing /etc/moxy/moxy.env is never overwritten.
set -euo pipefail

REPO="njyeung/moxy"
VERSION="${MOXY_VERSION:-latest}"
BIN=/usr/local/bin/moxy
CONF_DIR=/etc/moxy
ENV_FILE="$CONF_DIR/moxy.env"
UNIT=/etc/systemd/system/moxy.service

die() { echo "error: $*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "run as root"
command -v curl >/dev/null || die "curl is required"
command -v systemctl >/dev/null || die "systemd is required"

case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

if [[ $VERSION == latest ]]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading moxy ($VERSION, linux-$arch)"
for f in "moxy-linux-$arch" moxy.env.example checksums.txt; do
  curl -fsSL -o "$tmp/$f" "$base/$f" || die "could not download $base/$f"
done
(cd "$tmp" && sha256sum --check --ignore-missing --quiet checksums.txt) || die "checksum mismatch"

install -m 755 "$tmp/moxy-linux-$arch" "$BIN"
echo "installed $BIN"

install -d -m 700 "$CONF_DIR"
new_env=false
if [[ ! -f $ENV_FILE ]]; then
  install -m 600 "$tmp/moxy.env.example" "$ENV_FILE"
  new_env=true
  echo "created $ENV_FILE"
fi

cat >"$UNIT" <<'EOF'
[Unit]
Description=Moxy: forward VPS ports to Proxmox VMs
Documentation=https://github.com/njyeung/moxy
After=network-online.target wg-quick@wg0.service pveproxy.service
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
EnvironmentFile=/etc/moxy/moxy.env
ExecStart=/usr/local/bin/moxy
Restart=on-failure
RestartSec=10s
NoNewPrivileges=yes
ProtectSystem=strict
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable moxy.service >/dev/null

if $new_env; then
  cat <<EOF

Next steps:
  1. Fill in $ENV_FILE (see https://github.com/$REPO#env)
  2. systemctl start moxy
  3. journalctl -u moxy -f
EOF
else
  systemctl restart moxy.service
  echo "restarted moxy; logs: journalctl -u moxy -f"
fi
