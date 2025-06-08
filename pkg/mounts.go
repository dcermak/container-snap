package containersnap

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/moby/sys/mountinfo"
)

// FindDefaultSubvolume finds the default btrfs subvolume for / and returns its numeric ID
func FindDefaultSubvolume() (uint64, error) {
	f, err := os.Open("/proc/thread-self/mountinfo")
	defer f.Close()
	if err != nil {
		return 0, err
	}

	return findDefaultSubvolume(f)
}

func findDefaultSubvolume(r io.Reader) (uint64, error) {
	// find the mount for /
	mounts, err := mountinfo.GetMountsFromReader(r, func(i *mountinfo.Info) (skip bool, stop bool) {
		if i.Mountpoint == "/" {
			// don't skip, but stop, we found our match
			return false, true
		}
		// default: skip, but don't stop
		return true, false
	})
	if err != nil {
		return 0, err
	}
	if len(mounts) != 1 {
		return 0, errors.New("Did not find a unique root mount")
	}

	subVolMount := mounts[0]

	// we now need to extract the vfs options for the default btrfs
	// subvolume, they look like the following example:
	// rw,discard=async,space_cache=v2,subvolid=269,subvol=/@/var/lib/container-snapshots/btrfs/subvolumes/badbf16094bfd8100fdcd490d58e436f8527cea3c8841b42bd11be25047616e2
	// we really only care about the subvol & subvolid (actually only about
	// the subvolid)
	var subVolId, subVol string

	for _, option := range strings.Split(subVolMount.VFSOptions, ",") {
		optWithEqual := strings.Split(option, "=")
		if len(optWithEqual) != 2 {
			continue
		}
		switch optWithEqual[0] {
		case "subvolid":
			subVolId = optWithEqual[1]
		case "subvol":
			subVol = optWithEqual[1]
		}
	}
	if subVolId == "" || subVol == "" {
		return 0, errors.New("Could not infer default subvolume from mount options: " + subVolMount.VFSOptions)
	}
	id, err := strconv.Atoi(subVolId)
	return uint64(id), err
}
