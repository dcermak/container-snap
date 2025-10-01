# `container-snap`

**WARNING**

This is experimental software! Don't use it in production unless you're
comfortable salvaging a broken system.


`container-snap` is an experimental implementation of bootable containers. It
utilizes podman's btrfs storage driver to create subvolumes that contain the
container image rootfs.

`container-snap` is intended to be used in conjunction with `tukit`, which is
part of openSUSE's
[`transactional-update`](https://github.com/openSUSE/transactional-update). The
current command line interface of `container-snap` is built to be called by
`tukit`.


## Technical details

The btrfs storage driver of podman (and docker for that matter) stores container
images as btrfs subvolumes in the subdirectory
`$GraphRoot/btrfs/subvolumes/`. We "exploit" behavior by pulling down a
"bootable" container and let the storage driver create a btrfs subvolume with
its contents. Then we only have to switch the default subvolume of "/" to the
snapshot belonging to the container image and we're set (sorta-kinda).


## Conventions used

`container-snap` refers to images by their image `ID`, i.e. the hash obtained
via `podman inspect -f "{{.Id}}" $url`. This value is stable irrespective of the
underlying storage driver.

The btrfs driver stores each image layer as a subvolume by the layer
hash.


## btrfs snapshots

`tukit` is built around the creation of new snapshots from existing ones,
executing commands (usually updating the system) and switching the default
snapshot. Hence we have to provide similar functionality.

`container-snap` creates a new snaps


# Bootable Container Image Requirements

A container that is supposed to be used as the next snapshot must fulfill the
following requirements:

- `container-snap` must be present in the image under `/usr/bin/container-snap`

- `snapper` must **not** be installed (otherwise `transactional-update`/`tukit`
  will default to `snapper`)

- `transactional-update` must be installed in the image and must include
  [transactional-update#137](https://github.com/openSUSE/transactional-update/pull/137).

- a kernel must be present as well as systemd, grub2, rsync and a network stack


# Usage

On a machine with `container-snap` and `transactional-update` installed, first
pull down an image, e.g.:
```ShellSession
# container-snap pull registry.opensuse.org/home/dancermak/containers/opensuse/bootable:latest
```

Ensure that `snapper` is not present on the system by de-installing it:
```ShellSession
# zypper -n rm snapper
```

Now switch to the image that you just pulled down. You can use the convenience
script
[`container-snap-switch-snapshot.sh`](./container-snap-switch-snapshot.sh):

```ShellSession
# container-snap list-images
43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74,2025-01-09 10:47:31.818647723 +0000 UTC,registry.opensuse.org/home/dancermak/containers/opensuse/bootable:latest
# container-snap-switch-snapshot 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74
````

The script performs currently a few ugly but necessary steps (like copying
`/etc/fstab`, `/etc/resolv.conf` and `/etc/shadow`, reinstalling the bootloader
and running `dracut`).


# Give it a try

You can find packages for openSUSE in my home project on OBS, including:

- container-snap:
  https://build.opensuse.org/package/show/home:dancermak/container-snap

- `transactional-update` built with the necessary patches applied:
  https://build.opensuse.org/package/show/home:dancermak/transactional-update

- a "bootable" openSUSE container (available for Tumbleweed and Leap/SLE 15.6):
  https://build.opensuse.org/package/show/home:dancermak/opensuse-boot-image

- a fat "bootable" openSUSE container (derives from `opensuse-boot-image` and
  adds a few packages on top):
  https://build.opensuse.org/package/show/home:dancermak/opensuse-fat-boot-image

This repository contains a kiwi disk image description in the `images/`
subdirectory. To build the test image, run the following command from the
`images` subdirectory:

```bash
kiwi system boxbuild --box=tumbleweed -- --description . --target-dir /var/tmp/kiwi/
```

You will need kiwi and the kiwi boxbuild plugin for that. You can build the
image using plain kiwi without the boxbuild plugin, but note that this might not
work if your root partition is using btrfs (btrfs has issues with nested
subvolumes).

The resulting disk image will be in
`/var/tmp/kiwi/container-snap-system.x86_64-1.0.0-0.qcow2` and can be booted
directly from. It will boot into a minimal openSUSE Tumbleweed that on first
boot will load the
[`opensuse-boot-image`](https://build.opensuse.org/package/show/home:dancermak/opensuse-boot-image)
and set to boot from it. When you reboot the VM, you'll be now running a system
using `container-snap`!
