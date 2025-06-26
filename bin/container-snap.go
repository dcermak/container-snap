package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/reexec"
	containersnap "github.com/dcermak/container-snap/pkg"

	logrus "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v3"
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

func main() {
	var ctrSnap *containersnap.ContainerSnap = nil
	var url, id string

	cmd := &cli.Command{
		Name:  "container-snap",
		Usage: "OCI image based snapshot creation utility",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Set verbosity level (-v, -vv, -vvv, etc.)",
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			log = logrus.New()
			log.SetLevel(logrus.Level(c.Count("verbose")))

			reexec.Init()
			var err error
			ctrSnap, err = containersnap.NewContainerSnap("/var/lib/container-snap")
			return ctx, err
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initialize container-snap's btrfs storage",
				Action: func(ctx context.Context, c *cli.Command) error {
					// everything is already setup in Before:
					return nil
				},
			},
			{
				Name:      "pull",
				Usage:     "Pull the supplied image",
				Arguments: []cli.Argument{&cli.StringArg{Name: "url", Destination: &url}},
				Action: func(ctx context.Context, c *cli.Command) error {
					imgs, err := ctrSnap.PullImage(url)
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
				Name:  "load",
				Usage: "Load the local image from archive or URL",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:        "url",
						Destination: &url,
						UsageText:   "URL to the image. Must be prefixed with a transport"},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "input",
						Aliases: []string{"i"},
						Usage:   "Path to the OCI archive from which the image should be loaded",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					var imageUrl string
					if inputFile := c.String("input"); inputFile != "" {
						imageUrl = "oci-archive://" + inputFile
					} else if url != "" {
						imageUrl = url
					} else {
						return fmt.Errorf("Either URL argument or --input/-i flag must be provided")
					}

					imgs, err := ctrSnap.PullImage(imageUrl)
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
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Destination: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					return ctrSnap.SwitchToSnapshot(containersnap.SnapshotId(id))
				},
			},
			{
				Name:      "get-root",
				Usage:     "Print the root directory of the image from the given ID",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Destination: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					root, err := ctrSnap.MountedImageDir(containersnap.SnapshotId(id))
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
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Destination: &id}},
				Flags: []cli.Flag{
					&cli.BoolWithInverseFlag{Name: "readonly"},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return ctrSnap.SetReadOnlyState(containersnap.SnapshotId(id), c.Bool("readonly"))
				},
			},
			{
				Name:      "get-readonly-state",
				Usage:     "get the readonly state of the snapshot with the given id",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Destination: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					roState, err := ctrSnap.GetReadOnlyState(containersnap.SnapshotId(id))
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
					images, err := ctrSnap.Images()
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
				Action: func(_ context.Context, c *cli.Command) error {
					snapshots, err := ctrSnap.AllContainerSnapshots()
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
					snapshot, err := ctrSnap.SnapshotFromBtrfsSubvolId(uint64(btrfsId))
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
					currentId, err := containersnap.FindDefaultSubvolume()
					if err != nil {
						return err
					}
					snapshot, err := ctrSnap.SnapshotFromBtrfsSubvolId(currentId)
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
					images, err := ctrSnap.Images()
					if err != nil {
						return err
					}

					if url := c.String("url"); url != "" {
						for _, img := range images {
							for _, imgName := range img.Names {
								if imgName == url {
									err := ctrSnap.DeleteImage(img.ID)
									if err != nil {
										return err
									}
								}
							}
						}
					} else if id := c.String("id"); id != "" {
						return ctrSnap.DeleteSnapshot(containersnap.SnapshotId(id))
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
					s, err := containersnap.GetSnapshot(containersnap.SnapshotId(id))
					if err != nil {
						return err
					}

					dir, err := ctrSnap.MountedImageDirFromSnapshot(*s)
					if err == nil {
						fmt.Println(dir)
					}
					return err
				},
			},
			{
				Name:      "create-snapshot",
				Usage:     "Creates a snapshot based on the provided ID and prints the new ID to stdout",
				Arguments: []cli.Argument{&cli.StringArg{Name: "id", Destination: &id}},
				Action: func(ctx context.Context, c *cli.Command) error {
					snap, err := ctrSnap.NewSnapshot(containersnap.SnapshotId(id))
					if err == nil {
						fmt.Println(snap.SnapshotId())
					}
					return err
				},
			},
		},
	}

	err := cmd.Run(context.Background(), os.Args)
	if err != nil {
		panic(err)
	}
}
