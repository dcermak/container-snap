#!/bin/sh

set -euxo pipefail

tukit -v call "$1" zypper -n in -f grub2
tukit -v call "$1" dracut --force --regenerate-all
tukit -v call "$1" bash -c "/usr/sbin/grub2-mkconfig > /boot/grub2/grub.cfg"
tukit -v call "$1" /sbin/pbl --install
