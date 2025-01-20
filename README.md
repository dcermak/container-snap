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

- `transactional-update` must be installed in the image

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

Now switch to the image that you just pulled down:

```ShellSession
# container-snap list-images
43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74,2025-01-09 10:47:31.818647723 +0000 UTC,registry.opensuse.org/home/dancermak/containers/opensuse/bootable:latest
# container-snap switch 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74
````

Now we have to perform a few ugly but necessary manual steps. First, we need to
copy our existing `/etc/fstab` into the snapshot so that the existing subvolume
setup is working in the snapshot (otherwise `/var/` will not be a subvolume and
a lot of assumptions of `tukit` will no longer apply):

```ShellSession
# cp /etc/fstab $(container-snap get-root 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74)/etc/
```

Also, you'll most likely need to do the same with `/etc/resolv.conf` or you
won't have any name resolution inside the new snapshot:
```ShellSession
# cp /etc/resolv.conf $(container-snap get-root 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74)/etc/
```

And finally, in case your container image does not specify an unprivileged user
or sets a root password, you definitely want to copy `/etc/shadow`, or you won't
be able to log in after reboot:
```ShellSession
# cp /etc/shadow $(container-snap get-root 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74)/etc/
```

Now we need to make sure that the system will actually boot from the new
snapshot. For that, we will reinstall grub2, regenerate the initrd, write the
grub config and force install the bootloader again. On openSUSE, this is run as
follows:

```ShellSession
# tukit call 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74 zypper -n in -f grub2
.. snip ..
# tukit call 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74 dracut --force --regenerate-all
.. snip ..
# tukit call 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74 bash -c "/usr/sbin/grub2-mkconfig > /boot/grub2/grub.cfg"
.. snip ..
# tukit call 43de4dcfccec5cd0b92c04afe1bbde645ff24bff5ff8845b73e82ae8bfd58e74 /sbin/pbl --install
.. snip ..
```

Now reboot and cross your fingers that the machine actually comes up again 🤞
