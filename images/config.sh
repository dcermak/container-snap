#!/bin/bash
#======================================
# Functions...
#--------------------------------------
test -f /.kconfig && . /.kconfig
test -f /.profile && . /.profile

#======================================
# Greeting
#--------------------------------------
echo "Configure image: [$kiwi_iname]..."

suseSetupProduct

#======================================
# Specify default systemd target
#--------------------------------------
baseSetRunlevel 3

#======================================
# Set hostname by DHCP
#--------------------------------------
baseUpdateSysConfig /etc/sysconfig/network/dhcp DHCLIENT_SET_HOSTNAME yes

#======================================
# Enable DHCP on eth0
#--------------------------------------
cat >/etc/sysconfig/network/ifcfg-eth0 <<EOF
BOOTPROTO='dhcp'
STARTMODE='auto'
EOF

# Add repos from /etc/YaST2/control.xml
if [ -x /usr/sbin/add-yast-repos ]; then
	add-yast-repos
	zypper --non-interactive rm -u live-add-yast-repos
fi

# import rpm keys, this is not done automatically for some reason
for key in /usr/lib/rpm/gnupg/keys/*; do
    rpm -import "$key"
done

systemctl enable NetworkManager.service
systemctl enable systemd-resolved.service

# convenience shell for libvirt
systemctl enable serial-getty@ttyS0.service

# Enable container-snap-first-boot service
systemctl enable container-snap-first-boot.service

# Create the first-boot-required file
mkdir -p /var/lib/container-snap
touch /var/lib/container-snap/.first-boot-required
