package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/containers/common/libimage"
	"github.com/containers/common/pkg/config"
	"github.com/containers/storage"

	//graphdriver "github.com/containers/storage/drivers"
	"github.com/containers/storage/pkg/reexec"

	logrus "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v3"

	btrfs "github.com/containerd/btrfs/v2"
)

var log *logrus.Logger

// getDefaultBtrfsSubvolume returns the numeric ID of the current default btrfs
// subvolume set for the root partition
func getDefaultBtrfsSubvolume() (string, error) {
	var out strings.Builder

	re := regexp.MustCompile(`^ID\s+([0-9]+)`)

	// output looks like this:
	// ID 272 gen 152 top level 257 path @/.snapshots/3/snapshot
	cmd := exec.Command("btrfs", "subvolume", "get-default", "/")
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	matches := re.FindStringSubmatch(out.String())
	// matches should be a slice of:
	// [$matchedString, $capturingGroup] or nil if there's no match
	if matches == nil || len(matches) != 2 {
		return "", fmt.Errorf("Invalid output from 'btrfs subvolume get-default /': %s", out.String())
	}

	return matches[1], nil
}

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

// func (c *ContainerSnap) images

// SnapshotId is the user-facing identifier of a snapshot of the form `$imgID`
// or `$imgID-work$N` (where `N` is a natural number)
type SnapshotId string

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

func (c *ContainerSnap) allContainerSnapshots() ([]containerSnapshot, error) {
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
		toplayerSnap, err := getSnapshot(SnapshotId(subVolInfo.Name))
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

func (c *ContainerSnap) snapshotFromBtrfsSubvolId(subvolId uint64) (*Snapshot, error) {
	snapshots, err := c.allContainerSnapshots()

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

func (s Snapshot) SnapshotId() SnapshotId {
	if s.SnapshotNumber == 0 {
		return SnapshotId(s.ImgId)
	}
	return SnapshotId(fmt.Sprintf("%s-work%d", s.ImgId, s.SnapshotNumber))
}

func getSnapshot(id SnapshotId) (*Snapshot, error) {
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

func (c *ContainerSnap) graphRoot() string {
	return c.store.GraphRoot()
}

func (c *ContainerSnap) mountedImageDirFromSnapshot(snapshot Snapshot) (string, error) {
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

func (c *ContainerSnap) mountedImageDir(id SnapshotId) (string, error) {
	snapshot, err := getSnapshot(id)
	if err != nil {
		return "", err
	}

	return c.mountedImageDirFromSnapshot(*snapshot)
}

func (c *ContainerSnap) switchToSnapshot(id SnapshotId) error {
	dir, err := c.mountedImageDir(id)
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

func (c *ContainerSnap) pullImage(url string) ([]*libimage.Image, error) {
	if !strings.HasPrefix(url, "docker://") {
		url = "docker://" + url
	}

	// events := c.runtime.EventChannel()

	imgs, err := c.runtime.Pull(context.Background(), url, config.PullPolicyAlways, nil)
	if err != nil {
		return nil, err
	}
	return imgs, nil
}

func (c *ContainerSnap) deleteSnapshot(id SnapshotId) error {
	s, err := getSnapshot(id)
	if err != nil {
		return err
	}
	if s.SnapshotNumber == 0 {
		_, err := c.store.DeleteImage(string(s.SnapshotId()), true)
		return err
	}

	dir, err := c.mountedImageDirFromSnapshot(*s)
	if err != nil {
		return err
	}

	// SubvolDelete barfs on trailing /
	return btrfs.SubvolDelete(strings.TrimSuffix(dir, "/"))
}

func (c *ContainerSnap) setReadOnlyState(id SnapshotId, readonly bool) error {
	subvolPath, err := c.mountedImageDir(id)
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

func (c *ContainerSnap) getReadOnlyState(id SnapshotId) (bool, error) {
	subvolPath, err := c.mountedImageDir(id)
	if err != nil {
		return false, err
	}
	subvolInfo, err := btrfs.SubvolInfo(subvolPath)
	if err != nil {
		return false, err
	}
	return subvolInfo.Readonly, nil
}

func (c *ContainerSnap) newSnapshot(base SnapshotId) (*Snapshot, error) {
	baseSnapshot, err := getSnapshot(base)
	if err != nil {
		return nil, err
	}
	baseSubvolPath, err := c.mountedImageDirFromSnapshot(*baseSnapshot)
	if err != nil {
		return nil, err
	}

	newSnapshot := Snapshot{ImgId: baseSnapshot.ImgId, SnapshotNumber: baseSnapshot.SnapshotNumber + 1}
	newSnapshotPath, err := c.mountedImageDirFromSnapshot(newSnapshot)
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

func main() {
	log = logrus.New()
	// FIXME: set this via CLI
	log.SetLevel(logrus.TraceLevel)

	reexec.Init()

	ctrSnap, err := NewContainerSnap("/var/lib/container-snap")
	if err != nil {
		panic(err)
	}

	var url, id []string

	cmd := &cli.Command{
		Name:  "container-snap",
		Usage: "OCI image based snapshot creation utility",
		Commands: []*cli.Command{
			{
				Name:      "pull",
				Usage:     "Pull the supplied image",
				Arguments: []cli.Argument{&cli.StringArg{Name: "url", Min: 1, Max: 1, Values: &url}},
				Action: func(ctx context.Context, c *cli.Command) error {
					imgs, err := ctrSnap.pullImage(url[0])
					if err != nil {
						return err
					}

					if l := len(imgs); l != 1 {
						log.Errorf("Expected to pull exactly one image, but got %d", l)
					}

					for _, img := range imgs {
						log.Tracef("Pulled image with ID %s, saved as %s", img.ID(), img.TopLayer())
					}

					return nil
				},
			},
			{
				Name:      "switch",
				Usage:     "Switches the current default snapshot to the new one",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Min: 1, Max: 1, Values: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					return ctrSnap.switchToSnapshot(SnapshotId(id[0]))
				},
			},
			{
				Name:      "get-root",
				Usage:     "Print the root directory of the image from the given ID",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Min: 1, Max: 1, Values: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					root, err := ctrSnap.mountedImageDir(SnapshotId(id[0]))
					if err != nil {
						return err
					}
					fmt.Println(root)
					return nil
				},
			},
			{
				Name:      "set-readonly-state",
				Usage:     "set the snapshot with the given id to readonly or readwrite",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Min: 1, Max: 1, Values: &id}},
				Flags: []cli.Flag{
					&cli.BoolWithInverseFlag{
						BoolFlag: &cli.BoolFlag{
							Name: "readonly",
						},
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return ctrSnap.setReadOnlyState(SnapshotId(id[0]), c.Bool("readonly"))
				},
			},
			{
				Name:      "get-readonly-state",
				Usage:     "get the readonly state of the snapshot with the given id",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Min: 1, Max: 1, Values: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					roState, err := ctrSnap.getReadOnlyState(SnapshotId(id[0]))
					if err != nil {
						return err
					}

					if roState {
						fmt.Println("true")
					} else {
						fmt.Println("false")
					}

					return nil
				},
			},
			{
				Name:  "list-images",
				Usage: "Lists all container images in the storage",
				Action: func(_ context.Context, c *cli.Command) error {
					images, err := ctrSnap.store.Images()
					if err != nil {
						return err
					}
					for _, img := range images {
						imgName := "unknown-name"
						if len(img.Names) > 0 {
							imgName = img.Names[0]
						}
						fmt.Printf("%s,%s,%s\n", img.ID, img.Created, imgName)
					}
					return nil
				},
			},
			{
				Name:  "list-snapshots",
				Usage: "List all snapshots known to container-snap",
				Action: func(ctx context.Context, c *cli.Command) error {
					snapshots, err := ctrSnap.allContainerSnapshots()
					if err != nil {
						return err
					}
					fmt.Println("# snapshot-id,ro")
					for _, snapshot := range snapshots {
						fmt.Printf(
							"%s,%t\n",
							snapshot.SnapshotId(),
							//snapshot.subvolumeDir(ctrSnap.graphRoot(), snapshot.TopLayer),
							snapshot.SubvolInfo.Readonly,
						)
					}
					return nil
				},
			},
			{
				Name:  "get-default",
				Usage: "Print the default snapshot",
				Action: func(ctx context.Context, c *cli.Command) error {
					defaultId, err := getDefaultBtrfsSubvolume()
					if err != nil {
						return err
					}
					btrfsId, err := strconv.Atoi(defaultId)
					if err != nil {
						return err
					}
					snapshot, err := ctrSnap.snapshotFromBtrfsSubvolId(uint64(btrfsId))
					if err != nil {
						return err
					}
					fmt.Printf("%s\n", snapshot.SnapshotId())
					return nil
				},
			},
			{
				Name:  "get-current",
				Usage: "Print the currently active snapshot (i.e. the one from which the OS has been booted)",
				Action: func(ctx context.Context, c *cli.Command) error {
					// FIXME, this is wrong:
					defaultId, err := getDefaultBtrfsSubvolume()
					if err != nil {
						return err
					}
					btrfsId, err := strconv.Atoi(defaultId)
					if err != nil {
						return err
					}
					snapshot, err := ctrSnap.snapshotFromBtrfsSubvolId(uint64(btrfsId))
					if err != nil {
						return err
					}
					fmt.Printf("%s\n", snapshot.SnapshotId())
					return nil
				},
			},
			{
				Name:  "delete",
				Usage: "Delete an image by url or ID",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "url", Usage: "unique image url to delete"},
					&cli.StringFlag{Name: "id", Usage: "image id to delete"},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					images, err := ctrSnap.store.Images()
					if err != nil {
						return err
					}

					if url := c.String("url"); url != "" {
						for _, img := range images {
							for _, imgName := range img.Names {
								if imgName == url {
									_, err := ctrSnap.store.DeleteImage(img.ID, true)
									if err != nil {
										return err
									}
								}
							}
						}
					} else if id := c.String("id"); id != "" {
						return ctrSnap.deleteSnapshot(SnapshotId(id))
					} else {
						return fmt.Errorf("Either a url or an ID must be provided")
					}
					return nil
				},
			},
			{
				Name:  "get-root",
				Usage: "Prints the root directory of the specified snapshot",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "url", Usage: "unique image url to delete"},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					id := c.String("id")
					s, err := getSnapshot(SnapshotId(id))
					if err != nil {
						return err
					}

					dir, err := ctrSnap.mountedImageDirFromSnapshot(*s)
					if err == nil {
						fmt.Println(dir)
					}
					return err
				},
			},
			{
				Name:      "create-snapshot",
				Usage:     "Creates a snapshot based on the provided ID and prints the new ID to stdout",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Min: 1, Max: 1, Values: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					snap, err := ctrSnap.newSnapshot(SnapshotId(id[0]))
					if err == nil {
						fmt.Println(snap.SnapshotId())
					}
					return err
				},
			},
		},
	}

	err = cmd.Run(context.Background(), os.Args)
	if err != nil {
		panic(err)
	}
}
