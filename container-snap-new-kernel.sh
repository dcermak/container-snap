#!/bin/sh

set -euxo pipefail

tukit call "$1" zypper -n in -f grub2
tukit call "$1" dracut --force --regenerate-all
tukit call "$1" bash -c "/usr/sbin/grub2-mkconfig > /boot/grub2/grub.cfg"
tukit call "$1" /sbin/pbl --install
