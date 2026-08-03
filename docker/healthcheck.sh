#!/bin/bash
set -e
bash -c '</dev/tcp/127.0.0.1/1080'
if [ "$VPN_TYPE" = "ipsec" ]; then
    swanctl --list-sas 2>/dev/null | grep -q ESTABLISHED
else
    ip -o addr show | grep -Eq 'ppp[0-9]+|tun[0-9]+'
fi
