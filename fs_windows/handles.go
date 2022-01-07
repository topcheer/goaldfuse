// Copyright 2015 - 2017 Ka-Hing Cheung
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs_windows

import (
	"errors"
	"fmt"
	"github.com/billziss-gh/cgofuse/fuse"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type InodeAttributes2 struct {
	Size  uint64
	Mtime time.Time
}

func (i InodeAttributes2) Equal(other InodeAttributes2) bool {
	return i.Size == other.Size && i.Mtime.Equal(other.Mtime)
}

type Inode struct {
	Id           InodeID
	Name         string
	fs           *AliYunDriveFS
	Attributes   InodeAttributes2
	KnownSize    *uint64
	FileId       string
	ParentFileId string
	Type         string
	// It is generally safe to read `AttrTime` without locking because if some other
	// operation is modifying `AttrTime`, in most cases the reader is okay with working with
	// stale data. But Time is a struct and modifying it is not atomic. However
	// in practice (until the year 2157) we should be okay because
	// - Almost all uses of AttrTime will be about comparisions (AttrTime < x, AttrTime > x)
	// - Time object will have Time::monotonic bit set (until the year 2157) => the time
	//   comparision just compares Time::ext field
	// Ref: https://github.com/golang/go/blob/e42ae65a8507/src/time/time.go#L12:L56
	AttrTime time.Time

	mu sync.Mutex // everything below is protected by mu

	// We are not very consistent about enforcing locks for `Parent` because, the
	// parent field very very rarely changes and it is generally fine to operate on
	// stale parent informaiton
	Parent *Inode

	dir *DirInodeData

	Invalid     bool
	ImplicitDir bool

	fileHandles uint64

	// last known etag from the cloud
	knownETag *string
	// tell the next open to invalidate page cache because the
	// file is changed. This is set when LookUp notices something
	// about this file is changed
	invalidateCache bool

	// the refcnt is an exception, it's protected by the global lock
	// Goofys.mu
	refcnt uint64
}

func NewInode(fs *AliYunDriveFS, parent *Inode, name string) (inode *Inode) {
	if strings.Index(name, "/") != -1 {
		fmt.Println("%v is not a valid name", name)
	}

	inode = &Inode{
		Name:     name,
		fs:       fs,
		AttrTime: time.Now(),
		Parent:   parent,
		refcnt:   1,
	}

	return
}

func (inode *Inode) FullName() string {
	if inode.Parent == nil {
		return inode.Name
	} else {
		s := inode.Parent.getChildName(inode.Name)
		return s
	}
}

func (inode *Inode) touch() {
	inode.Attributes.Mtime = time.Now()
}

func (inode *Inode) InflateAttributes() (attr InodeAttributes) {
	mtime := inode.Attributes.Mtime
	if mtime.IsZero() {
		mtime = inode.fs.rootAttrs.Mtime
	}

	attr = InodeAttributes{
		Size:   inode.Attributes.Size,
		Atime:  mtime,
		Mtime:  mtime,
		Ctime:  mtime,
		Crtime: mtime,
		Uid:    inode.fs.flags.Uid,
		Gid:    inode.fs.flags.Gid,
	}

	if inode.dir != nil {
		attr.Nlink = 2
		attr.Mode = inode.fs.flags.DirMode | os.ModeDir
	} else {
		attr.Nlink = 1
		attr.Mode = inode.fs.flags.FileMode
	}
	return
}

func (inode *Inode) logFuse(op string, args ...interface{}) {
	fmt.Println(op, "[", args, "]")
}

func (inode *Inode) errFuse(op string, args ...interface{}) {
	fmt.Println(op, inode.Id, inode.FullName(), args)
}

func (inode *Inode) ToDir() {
	if inode.dir == nil {
		inode.Attributes = InodeAttributes2{
			Size: 4096,
			// Mtime intentionally not initialized
		}
		inode.dir = &DirInodeData{}
		inode.KnownSize = &inode.fs.rootAttrs.Size
	}
}

// LOCKS_REQUIRED(fs.mu)
// XXX why did I put lock required? This used to return a resurrect bool
// which no long does anything, need to look into that to see if
// that was legacy
func (inode *Inode) Ref() {
	inode.logFuse("Ref", inode.refcnt)

	inode.refcnt++
	return
}

func (inode *Inode) DeRef(n uint64) (stale bool) {
	inode.logFuse("DeRef", n, inode.refcnt)

	if inode.refcnt < n {
		panic(fmt.Sprintf("deref %v from %v", n, inode.refcnt))
	}

	inode.refcnt -= n

	stale = (inode.refcnt == 0)
	return
}

func (inode *Inode) GetAttributes() (*InodeAttributes, error) {
	// XXX refresh attributes
	//inode.logFuse("GetAttributes")
	if inode.Invalid {
		return nil, errors.New(strconv.FormatInt(int64(fuse.ENOENT), 10))
	}
	attr := inode.InflateAttributes()
	return &attr, nil
}

func (inode *Inode) isDir() bool {
	return inode.dir != nil
}

func (inode *Inode) OpenFile(metadata OpContext) (fh *FileHandle, err error) {
	inode.logFuse("OpenFile")

	inode.mu.Lock()
	defer inode.mu.Unlock()

	fh = NewFileHandle(inode)

	atomic.AddUint64(&inode.fileHandles, 1)
	return
}
