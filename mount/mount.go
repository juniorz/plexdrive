package mount

import (
	"os"

	"fmt"

	"strings"

	"strconv"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	. "github.com/claudetech/loggo/default"
	"github.com/dweidenfeld/plexdrive/chunk"
	"github.com/dweidenfeld/plexdrive/drive"
	"github.com/dweidenfeld/plexdrive/vfs"
)

func mountOptions(mountArgs []string) ([]fuse.MountOption, error) {
	// Set mount options
	options := []fuse.MountOption{
		fuse.NoAppleDouble(),
		fuse.NoAppleXattr(),
	}
	for _, option := range mountArgs {
		if "allow_other" == option {
			options = append(options, fuse.AllowOther())
		} else if "allow_root" == option {
			options = append(options, fuse.AllowRoot())
		} else if "allow_dev" == option {
			options = append(options, fuse.AllowDev())
		} else if "allow_non_empty_mount" == option {
			options = append(options, fuse.AllowNonEmptyMount())
		} else if "allow_suid" == option {
			options = append(options, fuse.AllowSUID())
		} else if strings.Contains(option, "max_readahead=") {
			data := strings.Split(option, "=")
			value, err := strconv.ParseUint(data[1], 10, 32)
			if nil != err {
				Log.Debugf("%v", err)
				return nil, fmt.Errorf("Could not parse max_readahead value")
			}
			options = append(options, fuse.MaxReadahead(uint32(value)))
		} else if "default_permissions" == option {
			options = append(options, fuse.DefaultPermissions())
		} else if "excl_create" == option {
			options = append(options, fuse.ExclCreate())
		} else if strings.Contains(option, "fs_name") {
			data := strings.Split(option, "=")
			options = append(options, fuse.FSName(data[1]))
		} else if "local_volume" == option {
			options = append(options, fuse.LocalVolume())
		} else if "writeback_cache" == option {
			options = append(options, fuse.WritebackCache())
		} else if strings.Contains(option, "volume_name") {
			data := strings.Split(option, "=")
			options = append(options, fuse.VolumeName(data[1]))
		} else if "read_only" == option {
			options = append(options, fuse.ReadOnly())
		} else {
			Log.Warningf("Fuse option %v is not supported, yet", option)
		}
	}

	return options, nil
}

// Mount the fuse volume
func Mount(
	client *drive.Client,
	chunkManager *chunk.Manager,
	mountpoint string,
	mountArgs []string,
	uid, gid uint32,
	umask os.FileMode) error {

	Log.Infof("Mounting path %v", mountpoint)

	if _, err := os.Stat(mountpoint); os.IsNotExist(err) {
		Log.Debugf("Mountpoint doesn't exist, creating...")
		if err := os.MkdirAll(mountpoint, 0644); nil != err {
			Log.Debugf("%v", err)
			return fmt.Errorf("Could not create mount directory %v", mountpoint)
		}
	}

	fuse.Debug = func(msg interface{}) {
		Log.Tracef("FUSE %v", msg)
	}

	mountOptions, err := mountOptions(mountArgs)
	if err != nil {
		return err
	}

	c, err := fuse.Mount(mountpoint, mountOptions...)
	if err != nil {
		return err
	}
	defer c.Close()

	filesys := vfs.NewFS(client, chunkManager, uid, gid, umask)
	if err := fs.Serve(c, filesys); err != nil {
		return err
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; nil != err {
		Log.Debugf("%v", err)
		return fmt.Errorf("Error mounting FUSE")
	}

	return Unmount(mountpoint, true)
}

// Unmount unmounts the mountpoint
func Unmount(mountpoint string, notify bool) error {
	if notify {
		Log.Infof("Unmounting path %v", mountpoint)
	}
	fuse.Unmount(mountpoint)
	return nil
}
