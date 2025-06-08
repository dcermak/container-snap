package containersnap

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	btrfs "github.com/containerd/btrfs/v2"

	"github.com/containers/common/libimage"
	"github.com/containers/common/pkg/config"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
)

// ContainerSnap is the main object of the container-snap binary
//
// `container-snap` dumps images into a btrfs store under `rootDir` where they
// appear as subvolumes.
//
// Images are found via their ID (that's `podman inspect -f {{.Id}} $url`) and
// new snapshots can be created from these. Snapshots are then saved as
// `/path/to/subvolume/$toplayerHash-work$N` where $N is a monotonically
// increasing counter.
type ContainerSnap struct {
	rootDir string

	store   storage.Store
	runtime *libimage.Runtime
}

func (c *ContainerSnap) AllContainerSnapshots() ([]containerSnapshot, error) {
	imgs, err := c.store.Images()
	if err != nil {
		return nil, err
	}

	snapMap := make(map[string]Snapshot, len(imgs))

	subvolumes, err := btrfs.SubvolList("/")
	if err != nil {
		return nil, err
	}

	snapshots := make([]containerSnapshot, 0, len(subvolumes))

	for _, img := range imgs {
		s := Snapshot{ImgId: img.ID, SnapshotNumber: 0}
		snapMap[img.TopLayer] = s
	}

	for _, subVolInfo := range subvolumes {
		// the subvolume name is the basename of the full path
		// i.e. if it's a container snapshot, then it has the form $toplayer or $toplayer-work$N

		// this is not the real snapshotId, as the id here is the toplayer and not the image ID
		toplayerSnap, err := GetSnapshot(SnapshotId(subVolInfo.Name))
		if err != nil {
			// not a "real" failure, all we know is that the name
			// doesn't follow our convention, so it doesn't belong
			// to container-snap
			continue
		}

		toplayer := toplayerSnap.ImgId
		s, found := snapMap[toplayer]
		if !found {
			// this occurs for snapshots where
			// `getSnapshot(subvolinfo.Name)` doesn't fail, because
			// the Name looks valid (e.g. it's just `var`), but the
			// name is not a tracked toplayer
			continue
		}

		snapshots = append(snapshots, containerSnapshot{
			// s is the parent, but we only take its image ID and
			// not the snapshot number, the snapshot number is taken
			// directly from the subvolume name (i.e. toplayerSnap)
			Snapshot:   Snapshot{ImgId: s.ImgId, SnapshotNumber: toplayerSnap.SnapshotNumber},
			TopLayer:   toplayer,
			SubvolInfo: subVolInfo,
		},
		)
	}

	return snapshots, nil
}

func (c *ContainerSnap) SnapshotFromBtrfsSubvolId(subvolId uint64) (*Snapshot, error) {
	snapshots, err := c.AllContainerSnapshots()

	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if snapshot.SubvolInfo.ID == subvolId {
			return &snapshot.Snapshot, nil
		}
	}
	return nil, fmt.Errorf("No matching snapshot with subvolume ID %d found", subvolId)
}

func (c *ContainerSnap) graphRoot() string {
	return c.store.GraphRoot()
}

func (c *ContainerSnap) Images() ([]storage.Image, error) {
	return c.store.Images()
}

func (c *ContainerSnap) DeleteImage(imgId string) error {
	_, err := c.store.DeleteImage(imgId, true)
	return err
}

func (c *ContainerSnap) MountedImageDirFromSnapshot(snapshot Snapshot) (string, error) {
	imgs, err := c.store.Images()
	if err != nil {
		return "", err
	}

	for _, img := range imgs {
		if snapshot.ImgId == img.ID {
			return snapshot.subvolumeDir(c.graphRoot(), img.TopLayer), nil
		}
	}
	return "", fmt.Errorf("No image with id '%s' found in image store", snapshot.ImgId)
}

func (c *ContainerSnap) MountedImageDir(id SnapshotId) (string, error) {
	snapshot, err := GetSnapshot(id)
	if err != nil {
		return "", err
	}

	return c.MountedImageDirFromSnapshot(*snapshot)
}

func (c *ContainerSnap) SwitchToSnapshot(id SnapshotId) error {
	dir, err := c.MountedImageDir(id)
	if err != nil {
		return err
	}

	cmd := exec.Command("btrfs", "subvolume", "set-default", dir)
	return cmd.Run()
}

func NewContainerSnap(rootDir string) (*ContainerSnap, error) {
	storeOptions := storage.StoreOptions{
		GraphDriverName: "btrfs",
		GraphRoot:       fmt.Sprintf("%s/snapshots", rootDir),
		RunRoot:         fmt.Sprintf("%s/runroot", rootDir),
	}

	store, err := storage.GetStore(storeOptions)
	if err != nil {
		return nil, err
	}

	runtime, err := libimage.RuntimeFromStore(store, nil)
	if err != nil {
		return nil, err
	}

	return &ContainerSnap{rootDir: rootDir, runtime: runtime, store: store}, nil
}

func (c *ContainerSnap) PullImage(url string) ([]*libimage.Image, error) {
	if !strings.HasPrefix(url, "docker://") {
		url = "docker://" + url
	}

	opts := libimage.PullOptions{CopyOptions: libimage.CopyOptions{Progress: make(chan types.ProgressProperties)}}
	go func() {
		for e := range opts.Progress {
			switch e.Event {
			case types.ProgressEventRead:
				fmt.Printf("Pulling layer %s, fetched %d of %d\n", e.Artifact.Digest, e.Offset, e.Artifact.Size)
			case types.ProgressEventDone:
				fmt.Printf("Pulled layer %s\n", e.Artifact.Digest)
			}
		}
	}()

	return c.runtime.Pull(context.Background(), url, config.PullPolicyAlways, &opts)
}

func (c *ContainerSnap) DeleteSnapshot(id SnapshotId) error {
	s, err := GetSnapshot(id)
	if err != nil {
		return err
	}
	if s.SnapshotNumber == 0 {
		_, err := c.store.DeleteImage(string(s.SnapshotId()), true)
		return err
	}

	dir, err := c.MountedImageDirFromSnapshot(*s)
	if err != nil {
		return err
	}

	// SubvolDelete barfs on trailing /
	return btrfs.SubvolDelete(strings.TrimSuffix(dir, "/"))
}

func (c *ContainerSnap) SetReadOnlyState(id SnapshotId, readonly bool) error {
	subvolPath, err := c.MountedImageDir(id)
	if err != nil {
		return err
	}

	roState := "false"
	if readonly {
		roState = "true"
	}
	cmd := exec.Command("btrfs", "property", "set", "-ts", subvolPath, "ro", roState)
	return cmd.Run()
}

func (c *ContainerSnap) GetReadOnlyState(id SnapshotId) (bool, error) {
	subvolPath, err := c.MountedImageDir(id)
	if err != nil {
		return false, err
	}
	subvolInfo, err := btrfs.SubvolInfo(subvolPath)
	if err != nil {
		return false, err
	}
	return subvolInfo.Readonly, nil
}

func (c *ContainerSnap) NewSnapshot(base SnapshotId) (*Snapshot, error) {
	baseSnapshot, err := GetSnapshot(base)
	if err != nil {
		return nil, err
	}
	baseSubvolPath, err := c.MountedImageDirFromSnapshot(*baseSnapshot)
	if err != nil {
		return nil, err
	}

	newSnapshot := Snapshot{ImgId: baseSnapshot.ImgId, SnapshotNumber: baseSnapshot.SnapshotNumber + 1}
	newSnapshotPath, err := c.MountedImageDirFromSnapshot(newSnapshot)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("btrfs", "subvolume", "snapshot", baseSubvolPath, newSnapshotPath)
	err = cmd.Run()

	if err != nil {
		return nil, err
	}
	return &newSnapshot, nil
}
