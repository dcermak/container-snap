package containersnap

import (
	"fmt"
	"strconv"
	"strings"

	btrfs "github.com/containerd/btrfs/v2"
)

// SnapshotId is the user-facing identifier of a snapshot of the form `$imgID`
// or `$imgID-work$N` (where `N` is a natural number)
type SnapshotId string

// Snapshot is the more developer friendly representation of a container-snap snapshot
type Snapshot struct {
	// ID of the base image
	ImgId string

	// monotonically increasing snapshot number
	SnapshotNumber int
}

type containerSnapshot struct {
	Snapshot
	TopLayer   string
	SubvolInfo btrfs.Info
}

// SnapshotId returns the SnapshotId of a Snapshot
func (s Snapshot) SnapshotId() SnapshotId {
	if s.SnapshotNumber == 0 {
		return SnapshotId(s.ImgId)
	}
	return SnapshotId(fmt.Sprintf("%s-work%d", s.ImgId, s.SnapshotNumber))
}

// GetSnapshot converts the SnapshotId id into a Snapshot struct.
func GetSnapshot(id SnapshotId) (*Snapshot, error) {
	sId := string(id)
	dashInd := strings.Index(sId, "-")

	if dashInd == -1 {
		return &Snapshot{ImgId: sId, SnapshotNumber: 0}, nil
	}

	tail := sId[dashInd+1:]
	numS, found := strings.CutPrefix(tail, "work")
	if !found {
		return nil, fmt.Errorf("snapshot id %s is invalid, tails doesn't end in 'work'", id)
	}
	num, err := strconv.Atoi(numS)
	if err != nil {
		return nil, err
	}
	return &Snapshot{ImgId: sId[:dashInd], SnapshotNumber: num}, nil
}

func (s Snapshot) subvolumeDir(graphRoot string, topLayerId string) string {
	baseDir := fmt.Sprintf("%s/btrfs/subvolumes/%s", graphRoot, topLayerId)
	if s.SnapshotNumber == 0 {
		return baseDir + "/"
	}
	return fmt.Sprintf("%s-work%d/", baseDir, s.SnapshotNumber)
}
