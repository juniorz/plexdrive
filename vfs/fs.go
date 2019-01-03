package vfs

import (
	"context"
	"fmt"
	"os"

	. "github.com/claudetech/loggo/default"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/dweidenfeld/plexdrive/chunk"
	"github.com/dweidenfeld/plexdrive/drive"
)

// FS the fuse filesystem
type FS struct {
	client       *drive.Client
	chunkManager *chunk.Manager
	uid          uint32
	gid          uint32
	umask        os.FileMode
}

func NewFS(client *drive.Client, chunkManager *chunk.Manager, uid uint32, gid uint32, umask os.FileMode) *FS {
	return &FS{
		client:       client,
		chunkManager: chunkManager,
		uid:          uid,
		gid:          gid,
		umask:        umask,
	}
}

// Root returns the root path
func (f *FS) Root() (fs.Node, error) {
	ctx := context.TODO()

	object, err := f.client.GetRoot(ctx)
	if nil != err {
		Log.Warningf("%v", err)
		return nil, fmt.Errorf("Could not get root object")
	}

	return &Object{
		client:       f.client,
		chunkManager: f.chunkManager,
		object:       object,
		uid:          f.uid,
		gid:          f.gid,
		umask:        f.umask,
	}, nil
}

// Object represents one drive object
type Object struct {
	client       *drive.Client
	chunkManager *chunk.Manager
	object       *drive.APIObject
	uid          uint32
	gid          uint32
	umask        os.FileMode
}

// Attr returns the attributes for a directory
func (o *Object) Attr(ctx context.Context, attr *fuse.Attr) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if o.object.IsDir {
		if o.umask > 0 {
			attr.Mode = os.ModeDir | o.umask
		} else {
			attr.Mode = os.ModeDir | 0755
		}
		attr.Size = 0
	} else {
		if o.umask > 0 {
			attr.Mode = o.umask
		} else {
			attr.Mode = 0644
		}
		attr.Size = o.object.Size
	}

	attr.Uid = uint32(o.uid)
	attr.Gid = uint32(o.gid)

	attr.Mtime = o.object.LastModified
	attr.Crtime = o.object.LastModified
	attr.Ctime = o.object.LastModified

	attr.Blocks = (attr.Size + 511) / 512

	return nil
}

// ReadDirAll shows all files in the current directory
func (o *Object) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	objects, err := o.client.GetObjectsByParent(o.object.ObjectID)
	if nil != err {
		Log.Debugf("%v", err)
		return nil, fuse.ENOENT
	}

	dirs := []fuse.Dirent{}
	for _, object := range objects {
		if object.IsDir {
			dirs = append(dirs, fuse.Dirent{
				Name: object.Name,
				Type: fuse.DT_Dir,
			})
		} else {
			dirs = append(dirs, fuse.Dirent{
				Name: object.Name,
				Type: fuse.DT_File,
			})
		}
	}
	return dirs, nil
}

// Lookup tests if a file is existent in the current directory
func (o *Object) Lookup(ctx context.Context, name string) (fs.Node, error) {
	object, err := o.client.GetObjectByParentAndName(o.object.ObjectID, name)
	if nil != err {
		Log.Tracef("%v", err)
		return nil, fuse.ENOENT
	}

	return &Object{
		client:       o.client,
		chunkManager: o.chunkManager,
		object:       object,
		uid:          o.uid,
		gid:          o.gid,
		umask:        o.umask,
	}, nil
}

// Read reads some bytes or the whole file
func (o *Object) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	response := make(chan chunk.Response)
	o.chunkManager.GetChunk(o.object, req.Offset, int64(req.Size), response)
	res := <-response

	if nil != res.Error {
		Log.Warningf("%v", res.Error)
		return fuse.EIO
	}

	resp.Data = res.Bytes[:]
	return nil
}

// Remove deletes an element
func (o *Object) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	obj, err := o.client.GetObjectByParentAndName(o.object.ObjectID, req.Name)
	if nil != err {
		Log.Warningf("%v", err)
		return fuse.EIO
	}

	err = o.client.Remove(ctx, obj, o.object.ObjectID)
	if nil != err {
		Log.Warningf("%v", err)
		return fuse.EIO
	}

	return nil
}

// Mkdir creates a new directory
func (o *Object) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	newObj, err := o.client.Mkdir(ctx, o.object.ObjectID, req.Name)
	if nil != err {
		Log.Warningf("%v", err)
		return nil, fuse.EIO
	}

	return &Object{
		client: o.client,
		object: newObj,
		uid:    o.uid,
		gid:    o.gid,
		umask:  o.umask,
	}, nil
}

// Rename renames an element
func (o *Object) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	obj, err := o.client.GetObjectByParentAndName(o.object.ObjectID, req.OldName)
	if nil != err {
		Log.Warningf("%v", err)
		return fuse.EIO
	}

	destDir, ok := newDir.(*Object)
	if !ok {
		Log.Warningf("%v", err)
		return fuse.EIO
	}

	err = o.client.Rename(ctx, obj, o.object.ObjectID, destDir.object.ObjectID, req.NewName)
	if nil != err {
		Log.Warningf("%v", err)
		return fuse.EIO
	}

	return nil
}
