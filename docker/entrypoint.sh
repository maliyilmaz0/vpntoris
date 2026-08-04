#!/bin/bash
set -eo pipefail

VPN_TYPE=${VPN_TYPE:-openvpn}
VPN_CONFIG=${VPN_CONFIG:-/vpn/config.conf} 
VPN_USER=${VPN_USER:-}
VPN_PASS=${VPN_PASS:-}
VPN_2FA=${VPN_2FA:-false}
VPN_HOST=${VPN_HOST:-}
VPN_PORT=${VPN_PORT:-443}
VPN_NAME=${VPN_NAME:-vpn}
VPN_DNS_SERVERS=${VPN_DNS_SERVERS:-}

mkdir -p /logs

if [ -n "$VPN_DNS_SERVERS" ]; then
	printf 'no-resolv\nlisten-address=0.0.0.0\nport=53\n' > /etc/dnsmasq.d/vpntoris.conf
	for server in ${VPN_DNS_SERVERS//,/ }; do printf 'server=%s\n' "$server" >> /etc/dnsmasq.d/vpntoris.conf; done
	dnsmasq
fi

if [ "$VPN_2FA" = "true" ]; then
	mkdir -p /run/vpntoris
	[ -p /run/vpntoris/otp ] || mkfifo /run/vpntoris/otp
fi

echo "Starting VPN client for $VPN_TYPE..."

if [ "$VPN_TYPE" = "openvpn" ]; then
    OPENVPN_ARGS=(--config "$VPN_CONFIG")
    if [ -n "$VPN_USER" ]; then
        mkdir -p /run/vpntoris
        printf '%s\n%s\n' "$VPN_USER" "$VPN_PASS" > /run/vpntoris/openvpn.auth
        chmod 600 /run/vpntoris/openvpn.auth
        OPENVPN_ARGS+=(--auth-user-pass /run/vpntoris/openvpn.auth)
    fi
    openvpn "${OPENVPN_ARGS[@]}" > >(tee "/logs/${VPN_NAME}.log") 2>&1 &
    VPN_PID=$!
elif [ "$VPN_TYPE" = "openconnect" ]; then
    echo "$VPN_PASS" | openconnect "$VPN_HOST:$VPN_PORT" -u "$VPN_USER" --passwd-on-stdin > "/logs/${VPN_NAME}.log" 2>&1 &
elif [ "$VPN_TYPE" = "openfortivpn" ]; then
	if [ ! -e /dev/ppp ]; then
		mknod /dev/ppp c 108 0
		chmod 660 /dev/ppp
	fi
	OTP_INPUT=()
	exec 3</dev/null
	if [ "$VPN_2FA" = "true" ]; then
		exec 3<>/run/vpntoris/otp
	fi
	echo "Testing SSL cert for Fortinet..."
    timeout 4 openfortivpn "$VPN_HOST:$VPN_PORT" -u "$VPN_USER" -p "$VPN_PASS" > /tmp/cert.log 2>&1 || true
    CERT=$(grep "\--trusted-cert" /tmp/cert.log | head -n 1 | awk '{print $NF}')
    if [ -n "$CERT" ]; then
        echo "Auto-trusting FortiGate SSL cert: $CERT"
        openfortivpn "$VPN_HOST:$VPN_PORT" -u "$VPN_USER" -p "$VPN_PASS" "${OTP_INPUT[@]}" --trusted-cert "$CERT" <&3 > "/logs/${VPN_NAME}.log" 2>&1 &
    else
        echo "No cert error found, proceeding normally..."
        openfortivpn "$VPN_HOST:$VPN_PORT" -u "$VPN_USER" -p "$VPN_PASS" "${OTP_INPUT[@]}" <&3 > "/logs/${VPN_NAME}.log" 2>&1 &
    fi
elif [ "$VPN_TYPE" = "ipsec" ]; then
	echo "Starting strongSwan IPsec client..."
	/usr/lib/ipsec/charon > "/logs/${VPN_NAME}.log" 2>&1 &
	for i in {1..20}; do
		[ -S /var/run/charon.vici ] && break
		sleep 0.25
	done
	swanctl --load-all 2>&1 | tee -a "/logs/${VPN_NAME}.log"
	IKE_TIMEOUT=35
	[ "$VPN_2FA" = "true" ] && IKE_TIMEOUT=180
	echo "Starting IKE negotiation (${IKE_TIMEOUT} second timeout)..." | tee -a "/logs/${VPN_NAME}.log"
	swanctl --initiate --child net --timeout "$IKE_TIMEOUT" --loglevel 2 2>&1 | tee -a "/logs/${VPN_NAME}.log"
else
    echo "Unknown VPN_TYPE: $VPN_TYPE"
    exit 1
fi

echo "Waiting for VPN interface to get an IP address..."
VPN_IFACE=""
if [ "$VPN_TYPE" = "ipsec" ]; then
	VPN_IFACE=$(ip route show table 220 default | awk '{for (i=1; i<=NF; i++) if ($i == "src") {print $(i+1); exit}}')
	if [ -z "$VPN_IFACE" ]; then
		VPN_IFACE=$(ip -4 addr show dev eth0 | awk '/scope global/ {print $2}' | cut -d/ -f1 | tail -n 1)
	fi
fi
for i in {1..30}; do
	if [ -n "$VPN_IFACE" ]; then break; fi
    if [ "$VPN_TYPE" = "openvpn" ] && ! kill -0 "$VPN_PID" 2>/dev/null; then
        wait "$VPN_PID"
        exit $?
    fi
    IFACE=$(ip -o addr show | awk '{print $2}' | grep -oE 'ppp[0-9]+|tun[0-9]+' | head -n 1 || true)
    if [ -n "$IFACE" ]; then
        VPN_IFACE=$IFACE
        echo "Found VPN interface: $VPN_IFACE"
        break
    fi
    sleep 1
done

if [ -z "$VPN_IFACE" ]; then
    echo "Timeout waiting for VPN interface."
    exit 1
fi

echo "Generating Dante SOCKS5 config for interface $VPN_IFACE..."
export VPN_IFACE
envsubst < /etc/danted.conf.template > /etc/danted.conf

echo "Starting Dante SOCKS5 server on port 1080..."
exec danted -f /etc/danted.conf
