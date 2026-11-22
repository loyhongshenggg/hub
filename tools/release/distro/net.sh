#!/bin/sh
# Networking for this distribution's Docker, built to coexist with any other
# Docker on the machine.
#
# All WSL distributions share ONE network namespace. Two Docker defaults
# would break another Docker (a dev distro, Docker Desktop) there:
#   - dockerd deletes the shared "docker0" bridge when started with
#     --bridge=none, or with default-address-pools and no --bridge
#     (moby daemon/daemon_unix.go, initNetworkController);
#   - with iptables on, dockerd rewrites chains the other Docker relies on.
# So this Docker gets its own bridge (hubbr0, created here; daemon.json
# "bridge": "hubbr0" makes dockerd leave docker0 alone), runs with iptables
# off, and this script adds outbound NAT scoped to its own range, never
# flushing anything. Called by docker.service (hub-net.conf).
range=10.231.0.0/16
bridge=hubbr0
addr=10.231.255.1/24
ipt() { iptables "$@" 2>/dev/null; }
case "${1:-}" in
pre)
  ip link show "$bridge" >/dev/null 2>&1 || ip link add "$bridge" type bridge
  ip addr show dev "$bridge" | grep -q "${addr%/*}" || ip addr add "$addr" dev "$bridge"
  ip link set "$bridge" up
  ;;
up)
  sysctl -qw net.ipv4.ip_forward=1
  ipt -t nat -C POSTROUTING -s $range ! -d $range -j MASQUERADE || iptables -t nat -A POSTROUTING -s $range ! -d $range -j MASQUERADE
  ipt -C FORWARD -s $range -j ACCEPT || iptables -I FORWARD -s $range -j ACCEPT
  ipt -C FORWARD -d $range -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT ||
    iptables -I FORWARD -d $range -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  ;;
down)
  ipt -t nat -D POSTROUTING -s $range ! -d $range -j MASQUERADE
  ipt -D FORWARD -s $range -j ACCEPT
  ipt -D FORWARD -d $range -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
  ip link del "$bridge" 2>/dev/null
  ;;
*) echo "usage: net.sh pre|up|down" >&2; exit 2 ;;
esac
exit 0
