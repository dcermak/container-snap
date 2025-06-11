#!/bin/sh

set -euxo pipefail

container-snap switch "$1"
cp /etc/fstab "$(container-snap get-root $1)"/etc/
cp /etc/resolv.conf "$(container-snap get-root $1)"/etc/
cp /etc/shadow "$(container-snap get-root $1)"/etc/

container-snap-new-kernel.sh "$1"
