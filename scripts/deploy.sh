#!/bin/sh
# Copy a built ./machinemodule and hack.json to a Push 3 for a manual test.
# Usage: scripts/deploy.sh [push.local]
# The config (machinemodule-config.json) and data/ folder on the device are
# left alone.
set -e
HOST="${1:-push.local}"
DIR=/data/push-hack/hacks/machinemodule
[ -x ./machinemodule ] || { echo "run 'make' first"; exit 1; }
ssh root@"$HOST" "/etc/init.d/push-hack-machinemodule stop" || true
ssh ableton@"$HOST" "mkdir -p $DIR"
scp machinemodule hack.json ableton@"$HOST":$DIR/
ssh ableton@"$HOST" "chmod +x $DIR/machinemodule"
ssh root@"$HOST" "/etc/init.d/push-hack-machinemodule start" || echo "service not installed yet: install via the catalog once, then redeploy"
echo "log: /data/push-hack/logs/machinemodule.log"
