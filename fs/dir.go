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
//go:build !windows
// +build !windows

package fs

import (
	"errors"
	"fmt"
	"github.com/jacobsa/fuse"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/model"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

type DirInodeData struct {
	mountPrefix string

	// these 2 refer to readdir of the Children
	lastOpenDir     *DirInodeData
	lastOpenDirIdx  int
	seqOpenDirScore uint8
	DirTime         time.Time

	Children []*Inode
}

type DirHandleEntry struct {
	Name   string
	Inode  fuseops.InodeID
	Type   fuseutil.DirentType
	Offset fuseops.DirOffset
}

type DirHandle struct {
	inode *Inode

	mu sync.Mutex // everything below is protected by mu

	Marker        *string
	lastFromCloud *string
	done          bool
	// Time at which we started fetching child entries
	// from cloud for this handle.
	refreshStartTime time.Time
}

func NewDirHandle(inode *Inode) (dh *DirHandle) {
	dh = &DirHandle{inode: inode}
	return
}

func (inode *Inode) OpenDir() (dh *DirHandle) {
	inode.logFuse("OpenDir")
	dh = NewDirHandle(inode)
	inode.fs.mu.Lock()
	inode.fs.dirHandles[fuseops.HandleID(dh.inode.fileHandles)] = dh
	inode.fs.mu.Unlock()
	return
}

// Sorting order of entries in directories is slightly inconsistent between goofys
// and azblob, s3. This inconsistency can be a problem if the listing involves
// multiple pagination results. Call this instead of `cloud.ListBlobs` if you are
// paginating.
//
// Problem: In s3 & azblob, prefixes are returned with '/' => the prefix "2019" is
// returned as "2019/". So the list api for these backends returns "2019/" after
// "2019-0001/" because ascii("/") > ascii("-"). This is problematic for goofys if
// "2019/" is returned in x+1'th batch and "2019-0001/" is returned in x'th; Goofys
// stores the results as they arrive in a sorted array and expects backends to return
// entries in a sorted order.
// We cant just use ordering of s3/azblob because different cloud providers have
// different sorting strategies when it involes directories. In s3 "a/" > "a-b/".
// In adlv2 it is opposite.
//
// Solution: To deal with this our solution with follows (for all backends). For
// a single call of ListBlobs, we keep requesting multiple list batches until there
// is nothing left to list or the last listed entry has all characters > "/"
// Relavant test case: TestReadDirDash

// LOCKS_REQUIRED(dh.mu)
// LOCKS_EXCLUDED(dh.inode.mu)
// LOCKS_EXCLUDED(dh.inode.fs)
func (dh *DirHandle) ReadDir() (en []*DirHandleEntry, err error) {
	parent := dh.inode
	fs := parent.fs
	dirList, err := aliyun.GetList(fs.Config.Token, fs.Config.DriveId, parent.FileId)
	if err != nil {
		return nil, err
	}
	if len(dirList.Items) == 0 {
		return nil, err
	}
	en = make([]*DirHandleEntry, 0)
	//add dot
	c := &DirHandleEntry{Name: ".", Type: fuseutil.DT_Directory, Inode: parent.Id, Offset: 0}
	en = append(en, c)
	//add dotdot
	if parent.Parent != nil {
		p := &DirHandleEntry{Name: "..", Type: fuseutil.DT_Directory, Inode: parent.Parent.Id, Offset: 1}
		en = append(en, p)
	} else {
		p := &DirHandleEntry{Name: "..", Type: fuseutil.DT_Directory, Inode: 0, Offset: 1}
		en = append(en, p)
	}

	for i, item := range dirList.Items {
		var ety *Inode
		var typ fuseutil.DirentType
		if inode := parent.findChildUnlocked(item.Name, item.Type); inode != nil {
			now := time.Now()
			// don't want to update time if this
			// inode is setup to never expire
			if inode.AttrTime.Before(now) {
				inode.AttrTime = now
			}
			ety = inode
			if ety.Type == "folder" {
				typ = fuseutil.DT_Directory
			} else {
				typ = fuseutil.DT_File
			}
		} else {
			entry := NewInode(dh.inode.fs, dh.inode, item.Name)
			if item.Type == "folder" {
				entry.ToDir()
				entry.Type = "folder"
				typ = fuseutil.DT_Directory
			} else {
				entry.Attributes = InodeAttributes{Size: uint64(item.Size),
					Mtime: item.UpdatedAt}
				entry.Type = "file"
				typ = fuseutil.DT_File
			}
			entry.ParentFileId = item.ParentFileId
			entry.FileId = item.FileId
			fs.insertInode(parent, entry)
			ety = entry
		}
		e := &DirHandleEntry{Name: ety.Name, Inode: ety.Id, Type: typ, Offset: fuseops.DirOffset(uint64(i + 2))}
		en = append(en, e)
	}
	return en, nil

}

func (dh *DirHandle) CloseDir() error {
	return nil
}

// Recursively resets the DirTime for child directories.
// ACQUIRES_LOCK(inode.mu)
func (inode *Inode) resetDirTimeRec() {
	inode.mu.Lock()
	if inode.dir == nil {
		inode.mu.Unlock()
		return
	}
	inode.dir.DirTime = time.Time{}
	// Make a copy of the child nodes before giving up the lock.
	// This protects us from any addition/removal of child nodes
	// under this node.
	children := make([]*Inode, len(inode.dir.Children))
	copy(children, inode.dir.Children)
	inode.mu.Unlock()
	for _, child := range children {
		child.resetDirTimeRec()
	}
}

// ResetForUnmount resets the Inode as part of unmounting a storage backend
// mounted at the given inode.
// ACQUIRES_LOCK(inode.mu)
func (inode *Inode) ResetForUnmount() {
	if inode.dir == nil {
		panic(fmt.Sprintf("ResetForUnmount called on a non-directory. name:%v",
			inode.Name))
	}

	inode.mu.Lock()
	// First reset the cloud info for this directory. After that, any read and
	// write operations under this directory will not know about this cloud.
	inode.dir.mountPrefix = ""

	// Clear metadata.
	// Set the metadata values to nil instead of deleting them so that
	// we know to fetch them again next time instead of thinking there's
	// no metadata
	inode.Attributes = InodeAttributes{}
	inode.Invalid, inode.ImplicitDir = false, false
	inode.mu.Unlock()
	// Reset DirTime for recursively for this node and all its child nodes.
	// Note: resetDirTimeRec should be called without holding the lock.
	inode.resetDirTimeRec()

}

func (parent *Inode) findPath(path string, typ string) (inode *Inode) {
	dir := parent

	for dir != nil {
		if !dir.isDir() {
			return nil
		}

		idx := strings.Index(path, "/")
		if idx == -1 {
			return dir.findChild(path, typ)
		}
		dirName := path[0:idx]
		path = path[idx+1:]

		dir = dir.findChild(dirName, typ)
	}

	return nil
}

func (parent *Inode) findChild(name string, typ string) (inode *Inode) {
	parent.mu.Lock()
	defer parent.mu.Unlock()

	inode = parent.findChildUnlocked(name, typ)
	return
}

func (parent *Inode) findInodeFunc(name string) func(i int) bool {
	return func(i int) bool {
		return (parent.dir.Children[i].Name) >= name
	}
}

func (parent *Inode) findChildUnlocked(name string, typ string) (inode *Inode) {
	l := len(parent.dir.Children)
	if l == 0 {
		return
	}
	if typ == "" {
		typ = "folder"
		if val, ok := parent.fs.pnCache[parent.FileId+name+typ]; ok {
			// found
			inode = val
		} else {
			typ = "file"
			if val, ok := parent.fs.pnCache[parent.FileId+name+typ]; ok {
				inode = val
			}
		}
	} else {
		if val, ok := parent.fs.pnCache[parent.FileId+name+typ]; ok {
			// found
			inode = val
		}
	}

	return inode
}

func (parent *Inode) removeChildUnlocked(inode *Inode) {
	l := len(parent.dir.Children)
	if l == 0 {
		return
	}
	i := sort.Search(l, parent.findInodeFunc(inode.Name))
	if i >= l || parent.dir.Children[i].Name != inode.Name {
		panic(fmt.Sprintf("%v.removeName(%v) but child not found: %v",
			parent.FullName(), inode.Name, i))
	}

	copy(parent.dir.Children[i:], parent.dir.Children[i+1:])
	parent.dir.Children[l-1] = nil
	parent.dir.Children = parent.dir.Children[:l-1]

	if cap(parent.dir.Children)-len(parent.dir.Children) > 20 {
		tmp := make([]*Inode, len(parent.dir.Children))
		copy(tmp, parent.dir.Children)
		parent.dir.Children = tmp
	}
}

func (parent *Inode) removeChild(inode *Inode) {
	parent.mu.Lock()
	defer parent.mu.Unlock()

	parent.removeChildUnlocked(inode)
	return
}

func (parent *Inode) insertChild(inode *Inode) {
	parent.mu.Lock()
	defer parent.mu.Unlock()

	parent.insertChildUnlocked(inode)
}

func (parent *Inode) insertChildUnlocked(inode *Inode) {
	l := len(parent.dir.Children)
	if l == 0 {
		parent.dir.Children = []*Inode{inode}
		return
	}

	//i := sort.Search(l, parent.findInodeFunc(inode.Name))
	if _, ok := parent.fs.pnCache[parent.FileId+inode.Name+inode.Type]; !ok {
		// not found = new value is the biggest
		parent.dir.Children = append(parent.dir.Children, inode)
	} else {
		parent.errFuse("Duplicate insert", parent.Name, inode.Name, inode.Type)
	}
}

func (parent *Inode) LookUp(name string) (inode *Inode, err error) {
	parent.logFuse("Inode.LookUp", name)

	return
}

func (parent *Inode) getChildName(name string) string {
	if parent.Id == fuseops.RootInodeID {
		return name
	} else {
		return fmt.Sprintf("%v/%v", parent.FullName(), name)
	}
}

func (parent *Inode) Unlink(name string) (err error) {
	parent.logFuse("Unlink", name)

	inode := parent.findChildUnlocked(name, "file")
	if inode == nil {
		return fuse.ENOENT
	}

	aliyun.RemoveTrash(parent.fs.Config.Token, parent.fs.Config.DriveId, inode.FileId, inode.ParentFileId)

	parent.mu.Lock()
	defer parent.mu.Unlock()

	if inode != nil {
		parent.removeChildUnlocked(inode)
		inode.Parent = nil
	}

	return
}

func (parent *Inode) Create(
	name string, metadata fuseops.OpContext) (inode *Inode, fh *FileHandle) {

	parent.logFuse("Create", name)

	fs := parent.fs

	parent.mu.Lock()
	defer parent.mu.Unlock()

	now := time.Now()
	inode = NewInode(fs, parent, name)
	inode.Attributes = InodeAttributes{
		Size:  0,
		Mtime: now,
	}

	fh = NewFileHandle(inode)
	fh.dirty = true
	inode.fileHandles = 1

	parent.touch()

	return
}

func (parent *Inode) MkDir(
	name string) (inode *Inode, err error) {

	fs := parent.fs

	item := aliyun.MakeDir(fs.Config.Token, fs.Config.DriveId, name, parent.FileId)
	if reflect.DeepEqual(item, model.CreateModel{}) {
		return nil, fuse.EIO
	}
	parent.mu.Lock()
	defer parent.mu.Unlock()

	inode = NewInode(fs, parent, name)
	inode.ToDir()
	inode.FileId = item.FileId
	inode.ParentFileId = item.ParentFileId
	inode.touch()
	if parent.Attributes.Mtime.Before(inode.Attributes.Mtime) {
		parent.Attributes.Mtime = inode.Attributes.Mtime
	}

	return inode, nil
}

func appendChildName(parent, child string) string {
	if len(parent) != 0 {
		parent += "/"
	}
	return parent + child
}

func (parent *Inode) isEmptyDir() (isDir bool, err error) {
	file := aliyun.GetFileDetail(parent.fs.Config.Token, parent.fs.Config.DriveId, parent.FileId)
	if !reflect.DeepEqual(file, model.ListModel{}) && file.Type == "folder" {
		isDir = true
	} else {
		err = errors.New("Not a dir")
	}
	return isDir, err
}

func (parent *Inode) RmDir(name string) (err error) {
	parent.logFuse("Rmdir", name)
	inode := parent.findChildUnlocked(name, "folder")
	if inode == nil {
		return fuse.ENOENT
	}
	isDir, err := inode.isEmptyDir()
	if err != nil {
		return
	}
	// if this was an implicit dir, isEmptyDir would have returned
	// isDir = false
	if isDir {
		aliyun.RemoveTrash(parent.fs.Config.Token, parent.fs.Config.DriveId, inode.FileId, inode.ParentFileId)
	}

	// we know this entry is gone
	parent.mu.Lock()
	defer parent.mu.Unlock()

	inode = parent.findChildUnlocked(name, "folder")
	if inode != nil {
		parent.removeChildUnlocked(inode)
		inode.Parent = nil
	}

	return
}

// semantic of rename:
// rename("any", "not_exists") = ok
// rename("file1", "file2") = ok
// rename("empty_dir1", "empty_dir2") = ok
// rename("nonempty_dir1", "empty_dir2") = ok
// rename("nonempty_dir1", "nonempty_dir2") = ENOTEMPTY
// rename("file", "dir") = EISDIR
// rename("dir", "file") = ENOTDIR
func (parent *Inode) Rename(from string, newParent *Inode, to string, oldNode *Inode) (err error) {
	config := parent.fs.Config
	fileId := oldNode.FileId
	if parent.Id == newParent.Id {
		aliyun.ReName(config.Token, config.DriveId, to, oldNode.FileId)

	} else {
		aliyun.BatchFile(config.Token, config.DriveId, oldNode.FileId, newParent.FileId)
		if from != to {
			aliyun.ReName(config.Token, config.DriveId, to, oldNode.FileId)
		}
	}
	parent.removeChildUnlocked(oldNode)
	newNode := &Inode{Parent: newParent,
		Name:         to,
		FileId:       fileId,
		ParentFileId: newParent.FileId}

	newParent.insertChildUnlocked(newNode)
	return
}

// if I had seen a/ and a/b, and now I get a/c, that means a/b is
// done, but not a/
func (parent *Inode) isParentOf(inode *Inode) bool {
	return inode.Parent != nil && (parent == inode.Parent || parent.isParentOf(inode.Parent))
}

func sealPastDirs(dirs map[*Inode]bool, d *Inode) {
	for p, sealed := range dirs {
		if p != d && !sealed && !p.isParentOf(d) {
			dirs[p] = true
		}
	}
	// I just read something in d, obviously it's not done yet
	dirs[d] = false
}

func (parent *Inode) findChildMaxTime() time.Time {
	maxTime := parent.Attributes.Mtime

	for i, c := range parent.dir.Children {
		if i < 2 {
			// skip . and ..
			continue
		}
		if c.Attributes.Mtime.After(maxTime) {
			maxTime = c.Attributes.Mtime
		}
	}

	return maxTime
}
