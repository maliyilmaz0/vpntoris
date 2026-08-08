#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
GOARCH=${GOARCH:-amd64}
IMAGE=${IMAGE:-golang:1.26-bookworm}
WORKDIR=/work

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

echo "Running Linux native OpenVPN smoke in Docker ($IMAGE, $GOARCH)…"

docker run --rm --platform "linux/$GOARCH" \
  --cap-add=NET_ADMIN \
  --device /dev/net/tun \
  -v "$ROOT_DIR:$WORKDIR" \
  -w "$WORKDIR/vpntoris-tray" \
  -e CGO_ENABLED=0 \
  "$IMAGE" \
  bash -ec '
set -euo pipefail
export PATH="/usr/local/go/bin:$PATH"
apt-get update >/dev/null
apt-get install -y --no-install-recommends iproute2 ca-certificates python3 >/dev/null

go test ./internal/nativehelper ./internal/netbackend ./internal/runtimepaths -count=1

mkdir -p /tmp/vpntoris-smoke/bin /tmp/vpntoris-smoke/engines/linux-'"$GOARCH"'
go build -o /tmp/vpntoris-smoke/bin/vpntoris-native-helper ./cmd/vpntoris-native-helper
go build -o /tmp/vpntoris-smoke/bin/fake-openvpn ./internal/nativehelper/testdata/fake_openvpn

ENGINE_DIR=/tmp/vpntoris-smoke/engines/linux-'"$GOARCH"'/openvpn
mkdir -p "$ENGINE_DIR/bin"
cp /tmp/vpntoris-smoke/bin/fake-openvpn "$ENGINE_DIR/bin/openvpn"
chmod 755 "$ENGINE_DIR/bin/openvpn"
DIGEST=$(sha256sum "$ENGINE_DIR/bin/openvpn" | awk "{print \$1}")
cat >"$ENGINE_DIR/manifest.json" <<EOF
{
  "id": "openvpn",
  "protocol": "openvpn",
  "version": "fake-1",
  "os": "linux",
  "architecture": "'"$GOARCH"'",
  "executable": "openvpn/bin/openvpn",
  "sha256": "$DIGEST",
  "license": "test",
  "capabilities": ["tun", "userpass", "challenge", "split-route"]
}
EOF

mkdir -p /run/vpntoris-native /var/log/vpntoris /var/lib/vpntoris/engines
cp -R /tmp/vpntoris-smoke/engines/linux-'"$GOARCH"' /var/lib/vpntoris/engines/

ip tuntap add mode tun dev tun9 || true
ip link set tun9 up || true

/tmp/vpntoris-smoke/bin/vpntoris-native-helper daemon 0 /var/lib/vpntoris/engines &
HELPER_PID=$!
trap "kill $HELPER_PID 2>/dev/null || true; ip link del tun9 2>/dev/null || true" EXIT

for i in $(seq 1 50); do
  if [[ -S /run/vpntoris-native/helper.sock ]]; then
    break
  fi
  sleep 0.1
done
test -S /run/vpntoris-native/helper.sock

python3 - <<PY
import json, socket, time, subprocess, os, sys

sock_path = "/run/vpntoris-native/helper.sock"

def rpc(payload):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(5)
    s.connect(sock_path)
    s.sendall((json.dumps(payload) + "\n").encode())
    data = b""
    while True:
        chunk = s.recv(4096)
        if not chunk:
            break
        data += chunk
        if b"\n" in data or data.endswith(b"}"):
            break
    s.close()
    return json.loads(data.decode())

start = rpc({
    "action": "start",
    "profile": "profile-office",
    "protocol": "openvpn",
    "configuration": "client\ndev tun\nremote 203.0.113.10 1194\nproto udp\n",
    "username": "test-user",
    "password": "test-password",
    "routes": ["10.38.0.0/16", "10.68.236.0/24"],
})
print("start:", start)
assert start.get("state") in ("connecting", "connected"), start

connected = False
for _ in range(80):
    status = rpc({"action": "status", "profile": "profile-office"})
    print("status:", status)
    if status.get("state") == "connected" and status.get("interface") == "tun9":
        connected = True
        break
    if status.get("state") == "failed":
        raise SystemExit(f"session failed: {status}")
    time.sleep(0.1)
assert connected, "tunnel did not become connected"

routes = subprocess.check_output(["ip", "-4", "route", "show"], text=True)
print("routes:\n", routes)
assert "10.38.0.0/16" in routes and "dev tun9" in routes
assert "10.68.236.0/24" in routes
assert "default" not in [line.split()[0] for line in routes.splitlines() if "tun9" in line] or True
for line in routes.splitlines():
    if "tun9" in line:
        assert not line.startswith("default") and "0.0.0.0/0" not in line, line

stop = rpc({"action": "stop", "profile": "profile-office"})
print("stop:", stop)
assert stop.get("state") == "stopped"
time.sleep(0.3)
routes_after = subprocess.check_output(["ip", "-4", "route", "show"], text=True)
print("routes after stop:\n", routes_after)
assert "10.38.0.0/16" not in routes_after
assert "10.68.236.0/24" not in routes_after

start2 = rpc({
    "action": "start",
    "profile": "profile-office",
    "protocol": "openvpn",
    "configuration": "client\ndev tun\nremote 203.0.113.10 1194\nproto udp\n",
    "username": "test-user",
    "password": "test-password",
    "routes": ["10.38.0.0/16"],
})
for _ in range(80):
    status = rpc({"action": "status", "profile": "profile-office"})
    if status.get("state") == "connected":
        break
    time.sleep(0.1)
reset = rpc({"action": "reset", "profile": "reset"})
print("reset:", reset)
time.sleep(0.3)
routes_reset = subprocess.check_output(["ip", "-4", "route", "show"], text=True)
assert "10.38.0.0/16" not in routes_reset
print("SMOKE OK")
PY
'

echo "Linux native OpenVPN smoke passed."
