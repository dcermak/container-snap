package containersnap

import (
	"fmt"
	"strings"
	"testing"
)

func TestObtainDefaultSubvolume(t *testing.T) {
	wellKnownMountInfo := `68 2 0:36 /@/var/lib/container-snapshots/btrfs/subvolumes/badbf16094bfd8100fdcd490d58e436f8527cea3c8841b42bd11be25047616e2 / rw,relatime shared:1 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=269,subvol=/@/var/lib/container-snapshots/btrfs/subvolumes/badbf16094bfd8100fdcd490d58e436f8527cea3c8841b42bd11be25047616e2
37 68 0:6 /dev rw,nosuid shared:2 - devtmpfs devtmpfs rw,size=4096k,nr_inodes=248208,mode=755,inode64
38 37 0:25 /dev/shm rw,nosuid,nodev shared:3 - tmpfs tmpfs rw,inode64
39 37 0:26 /dev/pts rw,nosuid,noexec,relatime shared:4 - devpts devpts rw,gid=5,mode=620,ptmxmode=000
40 68 0:24 /sys rw,nosuid,nodev,noexec,relatime shared:5 - sysfs sysfs rw
41 40 0:7 / /sys/kernel/security rw,nosuid,nodev,noexec,relatime shared:6 - securityfs securityfs rw
43 40 0:28 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:7 - cgroup2 cgroup2 rw,nsdelegate,memory_recursiveprot
43 40 0:29 / /sys/fs/pstore rw,nosuid,nodev,noexec,relatime shared:8 - pstore pstore rw
44 40 0:30 / /sys/fs/bpf rw,nosuid,nodev,noexec,relatime shared:9 - bpf bpf rw,mode=700
45 68 0:23 / /proc rw,nosuid,nodev,noexec,relatime shared:10 - proc proc rw
46 68 0:27 /run rw,nosuid,nodev shared:11 - tmpfs tmpfs rw,size=403348k,nr_inodes=819200,mode=755,inode64
27 45 0:32 / /proc/sys/fs/binfmt_misc rw,relatime shared:12 - autofs systemd-1 rw,fd=30,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=943
28 40 0:8 / /sys/kernel/debug rw,nosuid,nodev,noexec,relatime shared:13 - debugfs debugfs rw
29 37 0:37 / /dev/hugepages rw,relatime shared:14 - hugetlbfs hugetlbfs rw,pagesize=2M
30 37 0:21 / /dev/mqueue rw,nosuid,nodev,noexec,relatime shared:15 - mqueue mqueue rw
31 40 0:13 / /sys/kernel/tracing rw,nosuid,nodev,noexec,relatime shared:16 - tracefs tracefs rw
33 68 0:38 /tmp rw,nosuid,nodev shared:17 - tmpfs tmpfs rw,nr_inodes=1048576,inode64
34 46 0:39 /run/credentials/systemd-.mount rw,nosuid,nodev,noexec,relatime,nosymfollow shared:18 - tmpfs tmpfs rw,size=1024k,nr_inodes=1024,mode=700,inode64,noswap
35 40 0:40 / /sys/kernel/config rw,nosuid,nodev,noexec,relatime shared:38 - configfs configfs rw
47 40 0:41 / /sys/fs/fuse/connections rw,nosuid,nodev,noexec,relatime shared:42 - fusectl fusectl rw
51 68 0:42 /@/boot/grub2/i386-pc /boot/grub2/i386-pc rw,relatime shared:44 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=265,subvol=/@/boot/grub2/i386-pc
49 68 0:43 /@/.snapshots /.snapshots rw,relatime shared:46 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=257,subvol=/@/.snapshots
54 68 0:44 /@/boot/grub2/x86_64-efi /boot/grub2/x86_64-efi rw,relatime shared:48 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=266,subvol=/@/boot/grub2/x86_64-efi
56 68 0:45 /@/opt /opt rw,relatime shared:50 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=260,subvol=/@/opt
58 68 0:46 /@/home /home rw,relatime shared:52 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=259,subvol=/@/home
61 68 0:47 /@/root /root rw,relatime shared:54 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=261,subvol=/@/root
64 68 0:48 /@/srv /srv rw,relatime shared:56 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=262,subvol=/@/srv
80 68 0:49 /@/usr/local /usr/local rw,relatime shared:58 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=264,subvol=/@/usr/local
83 68 0:50 /@/var /var rw,relatime shared:60 - btrfs /dev/vda3 rw,discard=async,space_cache=v2,subvolid=263,subvol=/@/var
66 68 254:2 / /boot/efi rw,relatime - vfat /dev/vda2 rw,fmask=0022,dmask=0022,codepage=437,iocharset=iso8859-1,shortname=mixed,errors=remount-ro
44 68 0:52 /run/credentials/systemd-.mount rw,nosuid,nodev,noexec,relatime,nosymfollow shared:39 - tmpfs tmpfs rw,size=1024k,nr_inodes=1024,mode=700,inode64,noswap
`
	r := strings.NewReader(wellKnownMountInfo)
	subvolId, err := findDefaultSubvolume(r)

	if err != nil {
		t.Fatal(err)
	}
	expectedSubVolId := uint64(269)
	if subvolId != expectedSubVolId {
		t.Fatal(fmt.Sprintf("Invalid subvolume id, expected %d, got %d", expectedSubVolId, subvolId))
	}
}
