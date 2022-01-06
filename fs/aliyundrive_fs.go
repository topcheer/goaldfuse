//go:build !windows
// +build !windows

package fs

import (
	"context"
	"fmt"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"github.com/jacobsa/syncutil"
	"goaldfuse/aliyun"
	. "goaldfuse/aliyun/common"
	"goaldfuse/aliyun/model"
	"os/user"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"
)

var aliLog = GetLogger("ali")
var log = GetLogger("main")
var fuseLog = GetLogger("fuse")
var TOTAL uint64
var USED uint64

func NewAliYunDriveFsServer(config model.Config) (fuse.Server, error) {
	fs := &AliYunDriveFs{
		Config: config,
	}
	TOTAL = 0
	USED = 0
	now := time.Now()
	fs.rootAttrs = InodeAttributes{
		Size:  4096,
		Mtime: now,
	}
	fs.umask = 0122
	fs.bufferPool = BufferPool{}.Init()
	fs.nextInodeID = fuseops.RootInodeID + 1
	fs.inodes = make(map[fuseops.InodeID]*Inode)
	fs.pnCache = make(map[string]*Inode)
	root := NewInode(fs, nil, "")
	root.Id = fuseops.RootInodeID
	root.ToDir()
	root.Attributes.Mtime = fs.rootAttrs.Mtime
	root.FileId = "root"
	root.ParentFileId = "root"
	fs.inodes[fuseops.RootInodeID] = root
	fs.addDotAndDotDot(root)
	fs.nextHandleID = 1
	fs.dirHandles = make(map[fuseops.HandleID]*DirHandle)
	fs.fileHandles = make(map[fuseops.HandleID]*FileHandle)
	//fs.replicators = Ticket{Total: 16}.Init()
	//fs.restorers = Ticket{Total: 20}.Init()
	fs.flags = &FlagStorage{
		DirMode:          0755,
		FileMode:         0644,
		Uid:              currentUid(),
		Gid:              currentGid(),
		DebugAliYunDrive: true,
		DebugFuse:        true,
	}
	go func(driveFs *AliYunDriveFs) {
		for {
			aliLog.Log("Refresh Token")
			time.Sleep(120 * time.Second)
			refreshResult := aliyun.RefreshToken(driveFs.Config.RefreshToken)
			if !reflect.DeepEqual(refreshResult, model.RefreshTokenModel{}) {
				driveFs.Config = model.Config{
					DriveId:      refreshResult.DefaultDriveId,
					RefreshToken: refreshResult.RefreshToken,
					Token:        refreshResult.AccessToken,
					ExpireTime:   time.Now().Unix() + refreshResult.ExpiresIn,
				}
			}
		}

	}(fs)
	return fuseutil.NewFileSystemServer(FusePanicLogger{Fs: fs}), nil
}

func currentUid() uint32 {
	user, err := user.Current()
	if err != nil {
		panic(err)
	}

	uid, err := strconv.ParseUint(user.Uid, 10, 32)
	if err != nil {
		panic(err)
	}

	return uint32(uid)
}

func currentGid() uint32 {
	user, err := user.Current()
	if err != nil {
		panic(err)
	}

	gid, err := strconv.ParseUint(user.Gid, 10, 32)
	if err != nil {
		panic(err)
	}

	return uint32(gid)
}

type AliYunDriveFs struct {
	fuseutil.NotImplementedFileSystem
	Config model.Config
	flags  *FlagStorage

	umask       uint32
	rootAttrs   InodeAttributes
	mu          syncutil.InvariantMutex
	nextInodeID fuseops.InodeID
	bufferPool  *BufferPool
	inodes      map[fuseops.InodeID]*Inode

	nextHandleID fuseops.HandleID
	dirHandles   map[fuseops.HandleID]*DirHandle

	fileHandles map[fuseops.HandleID]*FileHandle

	//replicators *Ticket
	//restorers   *Ticket

	forgotCnt uint32
	pnCache   map[string]*Inode
}

func (fs *AliYunDriveFs) allocateInodeId() (id fuseops.InodeID) {
	id = fs.nextInodeID
	fs.nextInodeID++
	return
}

// LOCKS_REQUIRED(fs.mu)
// LOCKS_REQUIRED(parent.mu)
func (fs *AliYunDriveFs) insertInode(parent *Inode, inode *Inode) {
	addInode := false
	if inode.Name == "." {
		inode.Id = parent.Id
	} else if inode.Name == ".." {
		inode.Id = fuseops.InodeID(fuseops.RootInodeID)
		if parent.Parent != nil {
			inode.Id = parent.Parent.Id
		}
	} else {
		if inode.Id != 0 {
			panic(fmt.Sprintf("inode id is set: %v %v", inode.Name, inode.Id))
		}
		inode.Id = fs.allocateInodeId()
		addInode = true
	}
	parent.insertChildUnlocked(inode)
	if addInode {
		fs.mu.Lock()
		fs.inodes[inode.Id] = inode
		fs.pnCache[parent.FileId+inode.Name+inode.Type] = inode
		fs.mu.Unlock()
		// if we are inserting a new directory, also create
		// the child . and ..
		if inode.isDir() {
			fs.addDotAndDotDot(inode)
		}
	}
}

func (fs *AliYunDriveFs) addDotAndDotDot(dir *Inode) {
	dot := NewInode(fs, dir, ".")
	dot.ToDir()
	dot.AttrTime = TIME_MAX
	fs.insertInode(dir, dot)

	dot = NewInode(fs, dir, "..")
	dot.ToDir()
	dot.AttrTime = TIME_MAX
	fs.insertInode(dir, dot)
}

func (h *AliYunDriveFs) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {

	h.mu.RLock()
	inode := h.getInodeOrDie(op.Inode)
	h.mu.RUnlock()

	attr, err := inode.GetAttributes()
	if err == nil {
		op.Attributes = *attr
		op.AttributesExpiration = time.Now().Add(h.flags.StatCacheTTL)
	}

	return nil
}
func (fs *AliYunDriveFs) getInodeOrDie(id fuseops.InodeID) (inode *Inode) {
	fs.mu.RLock()
	inode = fs.inodes[id]
	fs.mu.RUnlock()
	if inode == nil {
		panic(fmt.Sprintf("Unknown inode: %v", id))
	}

	return
}
func (fs *AliYunDriveFs) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {
	fs.mu.RLock()
	inode := fs.getInodeOrDie(op.Inode)
	fs.mu.RUnlock()

	attr, err := inode.GetAttributes()
	if err == nil {
		op.Attributes = *attr
		op.AttributesExpiration = time.Now().Add(fs.flags.StatCacheTTL)
	}
	return
}

func (fs *AliYunDriveFs) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) (err error) {

	fs.mu.RLock()
	inode := fs.getInodeOrDie(op.Inode)
	fs.mu.RUnlock()

	if inode.Parent != nil {
		inode.Parent.mu.Lock()
		defer inode.Parent.mu.Unlock()
	}
	stale := inode.DeRef(op.N)

	if stale {
		fs.mu.Lock()
		defer fs.mu.Unlock()

		delete(fs.inodes, op.Inode)
		fs.forgotCnt += 1

		if inode.Parent != nil {
			inode.Parent.removeChildUnlocked(inode)
		}
	}

	return
}

func (h *AliYunDriveFs) StatFS(ctx context.Context, op *fuseops.StatFSOp) (err error) {
	if TOTAL == 0 && USED == 0 {
		total, used := aliyun.GetBoxSize(h.Config.Token)

		TOTAL, err = strconv.ParseUint(total, 10, 64)
		if err != nil {
			return
		}
		USED, err = strconv.ParseUint(used, 10, 64)
		if err != nil {
			return
		}
	}

	op.BlockSize = 4096

	const INODES = 1 * 1000 * 1000 * 1000 // 1 billion
	op.Blocks = TOTAL / uint64(op.BlockSize)
	op.BlocksFree = (TOTAL - USED) / uint64(op.BlockSize)
	op.BlocksAvailable = (TOTAL - USED) / uint64(op.BlockSize)
	op.IoSize = 2 * 1024 * 1024 // 1MB
	op.Inodes = INODES
	op.InodesFree = INODES

	return nil
}

func (fs *AliYunDriveFs) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) (err error) {

	var inode *Inode
	defer func() { fuseLog.Debugf("<-- LookUpInode %v %v %v", op.Parent, op.Name, err) }()

	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.Parent)
	fs.mu.RUnlock()

	parent.mu.Lock()
	inode = parent.findChildUnlocked(op.Name, "")
	if inode != nil {
		inode.Ref()
	} else {
		parent.mu.Unlock()
		return fuse.ENOENT
	}
	parent.mu.Unlock()

	op.Entry.Child = inode.Id
	op.Entry.Attributes = inode.InflateAttributes()
	op.Entry.AttributesExpiration = time.Now().Add(fs.flags.StatCacheTTL)
	op.Entry.EntryExpiration = time.Now().Add(fs.flags.TypeCacheTTL)

	return nil
}

func (fs *AliYunDriveFs) MkDir(ctx context.Context, op *fuseops.MkDirOp) error {
	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.Parent)
	fs.mu.RUnlock()
	dir, err := parent.MkDir(op.Name)
	if err != nil {
		return err
	}
	parent.mu.Lock()
	fs.insertInode(parent, dir)
	parent.mu.Unlock()
	op.Entry.Child = dir.Id
	op.Entry.Attributes = dir.InflateAttributes()
	op.Entry.AttributesExpiration = time.Now().Add(fs.flags.StatCacheTTL)
	op.Entry.EntryExpiration = time.Now().Add(fs.flags.TypeCacheTTL)
	return nil
}

func (fs *AliYunDriveFs) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) (err error) {
	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.Parent)
	fs.mu.RUnlock()

	inode, fh := parent.Create(op.Name, op.OpContext)

	parent.mu.Lock()

	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.insertInode(parent, inode)

	parent.mu.Unlock()

	op.Entry.Child = inode.Id
	op.Entry.Attributes = inode.InflateAttributes()
	op.Entry.AttributesExpiration = time.Now().Add(fs.flags.StatCacheTTL)
	op.Entry.EntryExpiration = time.Now().Add(fs.flags.TypeCacheTTL)

	// Allocate a handle.
	handleID := fs.nextHandleID
	fs.nextHandleID++
	fs.mu.Lock()
	fs.fileHandles[handleID] = fh
	fs.mu.Unlock()
	op.Handle = handleID

	inode.logFuse("<-- CreateFile")

	return
}

func (fs *AliYunDriveFs) Rename(ctx context.Context, op *fuseops.RenameOp) (err error) {
	if op.OldName == "" {
		fuseLog.Log("Not Supported")
		return
	}
	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.OldParent)
	newParent := fs.getInodeOrDie(op.NewParent)
	fs.mu.RUnlock()

	// XXX don't hold the lock the entire time
	if op.OldParent == op.NewParent {
		parent.mu.Lock()
		defer parent.mu.Unlock()
	} else {
		// lock ordering to prevent deadlock
		if op.OldParent < op.NewParent {
			parent.mu.Lock()
			newParent.mu.Lock()
		} else {
			newParent.mu.Lock()
			parent.mu.Lock()
		}
		defer parent.mu.Unlock()
		defer newParent.mu.Unlock()
	}
	oldnode := parent.findChildUnlocked(op.OldName, "")
	err = parent.Rename(op.OldName, newParent, op.NewName, oldnode)
	if err != nil {
		if err == fuse.ENOENT {
			// if the source doesn't exist, it could be
			// because this is a new file and we haven't
			// flushed it yet, pretend that's ok because
			// when we flush we will handle the rename
			inode := parent.findChildUnlocked(op.OldName, "")
			if inode != nil && atomic.LoadUint64(&inode.fileHandles) != 0 {
				err = nil
			}
		}
	}
	if err == nil {
		inode := parent.findChildUnlocked(op.OldName, "")
		if inode != nil {
			inode.mu.Lock()
			defer inode.mu.Unlock()

			parent.removeChildUnlocked(inode)

			newNode := newParent.findChildUnlocked(op.NewName, "")
			if newNode != nil {
				// this file's been overwritten, it's
				// been detached but we can't delete
				// it just yet, because the kernel
				// will still send forget ops to us
				newParent.removeChildUnlocked(newNode)
				newNode.Parent = nil
			}

			inode.Name = op.NewName
			inode.Parent = newParent
			newParent.insertChildUnlocked(inode)
		}
	}

	return nil
}

func (fs *AliYunDriveFs) RmDir(ctx context.Context, op *fuseops.RmDirOp) (err error) {
	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.Parent)
	fs.mu.RUnlock()

	err = parent.RmDir(op.Name)
	parent.logFuse("<-- RmDir", op.Name, err)
	return
}

func (fs *AliYunDriveFs) Unlink(ctx context.Context, op *fuseops.UnlinkOp) (err error) {
	fs.mu.RLock()
	parent := fs.getInodeOrDie(op.Parent)
	fs.mu.RUnlock()

	err = parent.Unlink(op.Name)
	return
}

func (h *AliYunDriveFs) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) error {
	//TODO implement me
	h.mu.RLock()
	inode := h.getInodeOrDie(op.Inode)
	h.mu.RUnlock()
	inode.OpenDir()
	return nil
}

func (h *AliYunDriveFs) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	//TODO implement me

	// Find the handle.
	h.mu.RLock()
	dh := h.dirHandles[op.Handle]
	h.mu.RUnlock()

	if dh == nil {
		panic(fmt.Sprintf("can't find dh=%v", op.Handle))
	}
	dh.mu.Lock()
	defer dh.mu.Unlock()
	entries, err := dh.ReadDir()
	if err != nil {
		return err
	}
	dh.inode.errFuse("ReadDir", len(entries), "Offset:", op.Offset)
	if op.Offset >= fuseops.DirOffset(uint64(len(entries)-1)) {
		op.BytesRead = 0
		return nil
	} else {
		for i := op.Offset; ; i++ {
			if i == fuseops.DirOffset(uint64(len(entries))) {
				break
			}
			entry := entries[i]
			n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], makeDirEntry(entry))
			if n == 0 {
				break
			}
			dh.inode.errFuse("Entry", "Written:", n)
			dh.inode.logFuse("<-- ReadDir", entry.Name, entry.Offset)
			op.BytesRead += n

		}
	}

	return nil
}
func makeDirEntry(en *DirHandleEntry) fuseutil.Dirent {
	return fuseutil.Dirent{
		Name:   en.Name,
		Type:   en.Type,
		Inode:  en.Inode,
		Offset: en.Offset,
	}
}
func (fs *AliYunDriveFs) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) (err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dh := fs.dirHandles[op.Handle]
	err = dh.CloseDir()
	if err != nil {
		return err
	}

	fuseLog.Debugln("ReleaseDirHandle", dh.inode.FullName())

	delete(fs.dirHandles, op.Handle)

	return
}

func (fs *AliYunDriveFs) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) (err error) {
	fs.mu.RLock()
	in := fs.getInodeOrDie(op.Inode)
	fs.mu.RUnlock()

	fh, err := in.OpenFile(op.OpContext)
	if err != nil {
		return
	}

	fs.mu.Lock()

	handleID := fs.nextHandleID
	fs.nextHandleID++

	fs.fileHandles[handleID] = fh
	fs.mu.Unlock()

	op.Handle = handleID

	in.mu.Lock()
	defer in.mu.Unlock()

	// this flag appears to tell the kernel if this open should
	// use the page cache or not. "use" here means:
	//
	// read will read from cache
	// write will populate cache
	//
	// because we have one flag to control both behaviors, if an
	// object is updated out-of-band and we need to invalidate
	// cache, and we write to this object locally, subsequent read
	// will not read from cache
	//
	// see tests TestReadNewFileWithExternalChangesFuse and
	// TestReadMyOwnWrite*Fuse
	op.KeepPageCache = !in.invalidateCache
	fh.keepPageCache = op.KeepPageCache
	in.invalidateCache = false

	return
}

func (fs *AliYunDriveFs) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {

	fs.mu.RLock()
	fh := fs.fileHandles[op.Handle]
	fs.mu.RUnlock()

	op.BytesRead, err = fh.ReadFile(op.Offset, op.Dst)

	return
}

func (fs *AliYunDriveFs) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) (err error) {
	fs.mu.RLock()

	fh, ok := fs.fileHandles[op.Handle]
	if !ok {
		panic(fmt.Sprintf("WriteFile: can't find handle %v", op.Handle))
	}
	fs.mu.RUnlock()

	err = fh.WriteFile(op.Offset, op.Data)
	return
}

func (h *AliYunDriveFs) SyncFile(ctx context.Context, op *fuseops.SyncFileOp) error {

	return h.FlushFile(ctx, &fuseops.FlushFileOp{Inode: op.Inode,
		Handle:    op.Handle,
		OpContext: op.OpContext})
}

func (h *AliYunDriveFs) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {

	err := h.fileHandles[op.Handle].FlushFile()
	if err != nil {
		return err
	}

	return nil
}

func (fs *AliYunDriveFs) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) (err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fh := fs.fileHandles[op.Handle]
	fh.Release()

	fuseLog.Debugln("ReleaseFileHandle", fh.inode.FullName(), op.Handle, fh.inode.Id)

	delete(fs.fileHandles, op.Handle)

	// try to compact heap
	//fs.bufferPool.MaybeGC()
	return
}
