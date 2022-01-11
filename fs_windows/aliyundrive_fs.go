package fs_windows

import (
	"fmt"
	"github.com/billziss-gh/cgofuse/fuse"
	cmap "github.com/orcaman/concurrent-map"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/model"
	. "goaldfuse/common"
	"goaldfuse/utils"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var aliLog = GetLogger("ali")

func NewAliYunDriveFSHost(config model.Config) *fuse.FileSystemHost {
	fs := &AliYunDriveFS{Config: config}
	TOTAL = 0
	USED = 0
	fs.ino++
	fmt.Println(config)
	list, _ := aliyun.GetList(config.Token, config.DriveId, "")
	fs.root = newNode(0, fs.ino, fuse.S_IFDIR|00777, 0, 0, "root", "")
	fs.inodes = cmap.New()
	fs.inodes.Set("/", model.ListModel{Name: "Default", Type: "folder", FileId: "root", ParentFileId: ""})
	for _, item := range list.Items {
		if item.Type == "folder" {
			fs.makeNode("/"+item.Name, fuse.S_IFDIR|0777, 0, 4096, item.FileId, "root")
		} else {
			fs.makeNode("/"+item.Name, fuse.S_IFREG|0666, 0, item.Size, item.FileId, "root")

		}
	}
	fs.openmap = map[uint64]*node_t{}

	go func(driveFs *AliYunDriveFS) {
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
				err := ioutil.WriteFile(".refresh_token", []byte(refreshResult.RefreshToken), 0600)
				if err != nil {
					fmt.Println("Can't write token file, token will not be able to persist")
				}
				utils.AccessToken = refreshResult.AccessToken
				utils.DriveId = refreshResult.DefaultDriveId
			}
		}

	}(fs)
	//go func(fs *AliYunDriveFS) {
	//	fmt.Println("Prefetch the file system")
	//	for key, value := range fs.root.chld {
	//		fmt.Println("/" + key)
	//		go fs.walkfn("/"+key, fs.inodes["/"+key], value)
	//		time.Sleep(10 * time.Second)
	//	}
	//}(fs)
	host := fuse.NewFileSystemHost(fs)
	return host
}

func (fs *AliYunDriveFS) walkfn(path string, fi model.ListModel, node *node_t) {
	if fi.Type == "folder" {
		list, _ := aliyun.GetList(fs.Config.Token, fs.Config.DriveId, fi.FileId)
		for _, item := range list.Items {
			fmt.Println("Prefetch:", path+"/"+item.Name)
			if item.Type == "folder" {
				_, node, _ := fs.makeNode(path+"/"+item.Name, fuse.S_IFDIR|0777, 0, 4096, item.FileId, item.ParentFileId)
				go fs.walkfn(path+"/"+item.Name, item, node)
				time.Sleep(10 * time.Second)
			} else {
				fs.makeNode(path+"/"+item.Name, fuse.S_IFREG|0666, 0, item.Size, item.FileId, item.ParentFileId)
			}
		}
	}
}

type node_t struct {
	stat         fuse.Stat_t
	xatr         map[string][]byte
	chld         map[string]*node_t
	fileId       string
	parentFileId string
	parent       *node_t
	opencnt      int
	readBytes    int64
	_if          string
	_tf          string
	lock         sync.RWMutex
}

func newNode(dev uint64, ino uint64, mode uint32, uid uint32, gid uint32, fileId string, parentFileId string) *node_t {
	now := fuse.Now()
	self := node_t{
		stat: fuse.Stat_t{
			Dev:      dev,
			Ino:      ino,
			Mode:     mode,
			Nlink:    1,
			Uid:      uid,
			Gid:      gid,
			Atim:     now,
			Mtim:     now,
			Ctim:     now,
			Birthtim: now,
			Flags:    0,
			Blksize:  2 * 1024 * 1024, //2MB
		},
		xatr:         nil,
		chld:         nil,
		fileId:       fileId,
		parentFileId: parentFileId,
		opencnt:      0}
	if fuse.S_IFDIR == self.stat.Mode&fuse.S_IFMT {
		self.chld = map[string]*node_t{}
	}
	return &self
}
func split(path string) []string {
	return strings.Split(path, "/")
}
func (fs *AliYunDriveFS) lookupNode(path string, ancestor *node_t) (prnt *node_t, name string, node *node_t) {
	prnt = fs.root
	name = ""
	node = fs.root
	if path != "/" {
		if fs.inodes.Has(path) {
			l, _ := fs.inodes.Get(path)
			r := l.(model.ListModel)
			if r.Type == "folder" {
				list, _ := aliyun.GetList(fs.Config.Token, fs.Config.DriveId, r.FileId)
				for _, item := range list.Items {
					if !fs.inodes.Has(path + "/" + item.Name) {
						if item.Type == "folder" {
							fs.makeNode(path+"/"+item.Name, fuse.S_IFDIR|0777, 0, 4096, item.FileId, item.ParentFileId)
						} else {
							fs.makeNode(path+"/"+item.Name, fuse.S_IFREG|0666, 0, item.Size, item.FileId, item.ParentFileId)

						}
					}
				}
			}
		}
	}
	for _, c := range split(path) {
		if "" != c {
			if 255 < len(c) {
				panic(fuse.Error(-fuse.ENAMETOOLONG))
			}
			prnt, name = node, c
			if node == nil {
				return
			}
			node = node.chld[c]
			if nil != ancestor && node == ancestor {
				name = "" // special case loop condition
				return
			}
		}
	}
	return
}

func (self *AliYunDriveFS) makeNode(path string, mode uint32, dev uint64, size int64, fileId string, parentFileId string) (int, *node_t, *node_t) {
	prnt, name, node := self.lookupNode(path, nil)
	if nil == prnt {
		return -fuse.ENOENT, nil, nil
	}
	if nil != node {
		return -fuse.EEXIST, nil, nil
	}
	prnt.lock.Lock()
	defer prnt.lock.Unlock()
	self.ino++
	uid, gid, _ := fuse.Getcontext()
	node = newNode(dev, self.ino, mode, uid, gid, fileId, parentFileId)
	node.stat.Size = size
	prnt.chld[name] = node
	prnt.stat.Ctim = node.stat.Ctim
	prnt.stat.Mtim = node.stat.Ctim
	if fileId != "" && parentFileId != "" {
		fi := aliyun.GetFileDetail(self.Config.Token, self.Config.DriveId, fileId)
		self.lock.Lock()
		self.inodes.Set(path, fi)
		self.lock.Unlock()
		return 0, node, prnt
	}
	if !self.inodes.Has(path) && mode == fuse.S_IFDIR|0777 {
		dir := aliyun.MakeDir(self.Config.Token, self.Config.DriveId, name, prnt.fileId)
		node.fileId = dir.FileId
		node.parentFileId = dir.ParentFileId
		fi := aliyun.GetFileDetail(self.Config.Token, self.Config.DriveId, dir.FileId)
		self.lock.Lock()
		self.inodes.Set(path, fi)
		self.lock.Unlock()
		return 0, node, prnt
	}
	//if reflect.DeepEqual(self.inodes[path], model.ListModel{}) && mode == fuse.S_IFREG|0666 {
	//	file := aliyun.ContentHandle(nil, self.Config.Token, self.Config.DriveId, prnt.fileId, name, 0)
	//	node.fileId = file
	//	node.parentFileId = prnt.fileId
	//	fi := aliyun.GetFileDetail(self.Config.Token, self.Config.DriveId, file)
	//	self.inodes[path] = fi
	//}
	return 0, node, prnt
}

func (self *AliYunDriveFS) removeNode(path string, dir bool) int {
	prnt, name, node := self.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if !dir && fuse.S_IFDIR == node.stat.Mode&fuse.S_IFMT {
		return -fuse.EISDIR
	}
	if dir && fuse.S_IFDIR != node.stat.Mode&fuse.S_IFMT {
		return -fuse.ENOTDIR
	}
	if 0 < len(node.chld) {
		return -fuse.ENOTEMPTY
	}
	node.stat.Nlink--
	delete(prnt.chld, name)
	self.lock.Lock()
	defer self.lock.Unlock()
	self.inodes.Remove(path)
	aliyun.RemoveTrash(self.Config.Token, self.Config.DriveId, node.fileId, node.parentFileId)
	tmsp := fuse.Now()
	node.stat.Ctim = tmsp
	prnt.stat.Ctim = tmsp
	prnt.stat.Mtim = tmsp
	return 0
}

func (self *AliYunDriveFS) openNode(path string, dir bool) (int, uint64) {
	_, _, node := self.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT, ^uint64(0)
	}
	if !dir && fuse.S_IFDIR == node.stat.Mode&fuse.S_IFMT {
		return -fuse.EISDIR, ^uint64(0)
	}
	if dir && fuse.S_IFDIR != node.stat.Mode&fuse.S_IFMT {
		return -fuse.ENOTDIR, ^uint64(0)
	}
	node.opencnt++
	if !dir {
		node.readBytes = 0
	}
	if 1 == node.opencnt {
		self.openmap[node.stat.Ino] = node
	}
	return 0, node.stat.Ino
}

func (self *AliYunDriveFS) closeNode(fh uint64) int {
	node := self.openmap[fh]
	node.opencnt--
	if 0 == node.opencnt {
		delete(self.openmap, node.stat.Ino)
	}
	return 0
}

func (self *AliYunDriveFS) getNode(path string, fh uint64) *node_t {
	if ^uint64(0) == fh {
		_, _, node := self.lookupNode(path, nil)
		return node
	} else {
		return self.openmap[fh]
	}
}
func (fs *AliYunDriveFS) synchronize() func() {
	fs.lock.Lock()
	return func() {
		fs.lock.Unlock()
	}
}

type AliYunDriveFS struct {
	fuse.FileSystemBase
	//Ali API Config
	Config model.Config
	//Read Write Lock
	lock    sync.RWMutex
	root    *node_t
	ino     uint64
	inodes  cmap.ConcurrentMap
	pnCache map[string]*node_t
	openmap map[uint64]*node_t
}

var TOTAL uint64
var USED uint64

func (fs *AliYunDriveFS) allocateInodeId() (id uint64) {
	id = fs.ino
	fs.ino++
	return
}

// Init is called when the file system is created.
func (fs *AliYunDriveFS) Init() {
	fmt.Println("Init")
}

// Destroy is called when the file system is destroyed.
func (fs *AliYunDriveFS) Destroy() {
	fmt.Println("Destroyed")
}

// Statfs gets file system statistics.

func (fs *AliYunDriveFS) Statfs(path string, stat *fuse.Statfs_t) int {

	if TOTAL == 0 && USED == 0 {
		total, used := aliyun.GetBoxSize(fs.Config.Token)
		TOTAL, _ = strconv.ParseUint(total, 10, 64)
		USED, _ = strconv.ParseUint(used, 10, 64)
	}
	stat.Bsize = 4096

	const INODES = 1 * 1000 * 1000 * 1000 // 1 billion
	stat.Blocks = TOTAL / stat.Bsize
	stat.Bfree = (TOTAL - USED) / stat.Bsize
	stat.Bavail = (TOTAL - USED) / stat.Bsize
	stat.Fsid = 88
	stat.Files = INODES
	stat.Ffree = INODES
	stat.Favail = INODES
	stat.Frsize = stat.Bsize
	stat.Namemax = 1000 * 10

	return 0
}

// Mknod creates a file node.

func (fs *AliYunDriveFS) Mknod(path string, mode uint32, dev uint64) int {
	fs.makeNode(path, mode, fuse.S_IFREG|0666, 0, "", "")
	return 0
}

// Mkdir creates a directory.

func (fs *AliYunDriveFS) Mkdir(path string, mode uint32) int {
	fs.makeNode(path, fuse.S_IFDIR|0777, 0, 4096, "", "")
	return 0
}

// Unlink removes a file.

func (fs *AliYunDriveFS) Unlink(path string) int {
	fs.removeNode(path, false)
	return 0
}

// Rmdir removes a directory.

func (fs *AliYunDriveFS) Rmdir(path string) int {
	fs.removeNode(path, true)
	return 0
}

// Link creates a hard link to a file.

func (fs *AliYunDriveFS) Link(oldpath string, newpath string) int {
	return -fuse.ENOENT
}

// Symlink creates a symbolic link.

func (fs *AliYunDriveFS) Symlink(target string, newpath string) int {
	return -fuse.ENOENT
}

// Readlink reads the target of a symbolic link.

func (fs *AliYunDriveFS) Readlink(path string) (int, string) {
	return -fuse.EINVAL, ""
}

// Rename renames a file.

func (fs *AliYunDriveFS) Rename(oldpath string, newpath string) (errc int) {
	fmt.Println("rename")
	oldprnt, oldname, oldnode := fs.lookupNode(oldpath, nil)
	if nil == oldnode {
		return -fuse.ENOENT
	}
	newprnt, newname, newnode := fs.lookupNode(newpath, oldnode)
	if nil == newprnt {
		return -fuse.ENOENT
	}
	if "" == newname {
		// guard against directory loop creation
		return -fuse.EINVAL
	}
	if oldprnt == newprnt && oldname == newname {
		return 0
	}
	if oldprnt == newprnt && oldname != newname {
		aliyun.ReName(fs.Config.Token, fs.Config.DriveId, newname, oldnode.fileId)
	}
	if oldprnt != newprnt && oldname == newname {
		aliyun.BatchFile(fs.Config.Token, fs.Config.DriveId, oldnode.fileId, newprnt.fileId)
	}
	if oldprnt != newprnt && oldname != newname {
		aliyun.ReName(fs.Config.Token, fs.Config.DriveId, newname, oldnode.fileId)
		aliyun.BatchFile(fs.Config.Token, fs.Config.DriveId, oldnode.fileId, newprnt.fileId)
	}
	if nil != newnode {
		errc = fs.removeNode(newpath, fuse.S_IFDIR == oldnode.stat.Mode&fuse.S_IFMT)
		if 0 != errc {
			return errc
		}
	}
	delete(oldprnt.chld, oldname)
	newprnt.chld[newname] = oldnode
	return 0
}

// Chmod changes the permission bits of a file.

func (fs *AliYunDriveFS) Chmod(path string, mode uint32) int {
	fmt.Println("chmod")
	return 0
}

// Chown changes the owner and group of a file.

func (fs *AliYunDriveFS) Chown(path string, uid uint32, gid uint32) int {
	return 0
}

// Utimens changes the access and modification times of a file.

func (fs *AliYunDriveFS) Utimens(path string, tmsp []fuse.Timespec) int {
	fmt.Println("utimes", path, tmsp)
	_, _, node := fs.lookupNode(path, nil)
	node.stat.Atim = tmsp[0]
	node.stat.Mtim = tmsp[1]
	return 0
}

// Access checks file access permissions.

func (fs *AliYunDriveFS) Access(path string, mask uint32) int {
	fmt.Println("Access", path, mask)
	return 0
}

// Create creates and opens a file.
// The flags are a combination of the fuse.O_* constants.

func (fs *AliYunDriveFS) Create(path string, flags int, mode uint32) (int, uint64) {
	fmt.Println("create", path, flags, mode)
	fs.makeNode(path, fuse.S_IFREG|0666, 0, 0, "", "")
	return fs.openNode(path, false)
}

// Open opens a file.
// The flags are a combination of the fuse.O_* constants.

func (fs *AliYunDriveFS) Open(path string, flags int) (int, uint64) {
	return fs.openNode(path, false)
}

// Getattr gets file attributes.
func (fs *AliYunDriveFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	node := fs.getNode(path, fh)
	if nil == node {
		return -fuse.ENOENT
	}
	*stat = node.stat
	return 0
}

// Truncate changes the size of a file.

func (fs *AliYunDriveFS) Truncate(path string, size int64, fh uint64) int {
	return 0
}

// Read reads data from a file.

func (fs *AliYunDriveFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	_, name, node := fs.lookupNode(path, nil)
	if node.stat.Size == 0 {
		return -fuse.EINPROGRESS
	}
	if node._if != "" {
		if _, err := os.Stat(node._if); err != nil {
			fmt.Println("Upload in progress", path, name, node._if)
			return -fuse.EINPROGRESS
		}
	}
	fi := aliyun.GetFileDetail(fs.Config.Token, fs.Config.DriveId, node.fileId)
	if fi.DownloadUrl == "" {
		fmt.Println("No Download URL")
		return fuse.ENOENT
	}
	var rangeStr = "bytes=0-"
	//if ofst > fi.Size {
	//	return 0
	//}
	if ofst+int64(len(buff)) < fi.Size {
		rangeStr = "bytes=" + strconv.FormatInt(ofst, 10) + "-" + strconv.FormatUint(uint64(ofst+int64(len(buff))), 10)
	} else if fi.Size < int64(len(buff)) {
		rangeStr = "bytes=" + strconv.FormatInt(ofst, 10) + "-" + strconv.FormatInt(fi.Size, 10)
	} else {
		rangeStr = "bytes=" + strconv.FormatInt(ofst, 10) + "-"
	}
	//if node.readBytes != ofst {
	//	node.readBytes = 0
	//	return int(fi.Size - ofst)
	//}

	//create temp file for performance consideration
	//if node._tf == "" {
	//	now := time.Now()
	resp := aliyun.GetFile(fi.DownloadUrl, fs.Config.Token, rangeStr)
	//	tempFile, _ := os.OpenFile(os.TempDir()+"/_tf"+name+strconv.FormatUint(node.stat.Ino, 10), os.O_RDWR|os.O_CREATE, 0666)
	//	io.Copy(tempFile, resp.Body)
	//	tempFile.Close()
	//	fmt.Println("Download Completed:" + time.Now().Sub(now).String())
	//	node._tf = os.TempDir() + "/_tf" + name + strconv.FormatUint(fh, 10)
	//}
	//
	//tempFile, eo := os.OpenFile(os.TempDir()+"/_tf"+name+strconv.FormatUint(node.stat.Ino, 10), os.O_RDONLY, 0666)
	//if eo != nil {
	//	fmt.Println("can't open cache file", node._tf)
	//	return 0
	//}
	//if ofst != 0 {
	//	seek, err := tempFile.Seek(ofst, 0)
	//	if err != nil {
	//		fmt.Println("seek err", err)
	//		return 0
	//	} else {
	//		fmt.Println("current ofst in file", seek)
	//	}
	//}
	//defer func(tempFile *os.File) {
	//	err := tempFile.Close()
	//	if err != nil {
	//		fmt.Println("can't close cache file", node._tf)
	//	}
	//}(tempFile)
	//byteRead := 0
	fmt.Println("Read", node.readBytes, "total", fi.Size, "off", ofst, "bufsize", len(buff))
	full, err := io.ReadFull(resp.Body, buff)
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			fmt.Println("Reach EOF")
			return full
		} else {
			fmt.Println("IO Error", err)
			return -fuse.EIO
		}

	}
	node.readBytes += int64(full)
	ofst += int64(full)
	return full

}

// Write writes data to a file.

func (fs *AliYunDriveFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	//fmt.Println("Write:", len(buff), ofst)
	_, name, node := fs.lookupNode(path, nil)
	var tempFilename = name + strconv.FormatUint(fh, 10)
	node.lock.Lock()
	defer node.lock.Unlock()
	var intermediateFile *os.File
	var err error

	intermediateFile, err = os.OpenFile(os.TempDir()+"/"+tempFilename, os.O_RDWR|os.O_CREATE, 0666)
	node._if = intermediateFile.Name()
	if err != nil {
		fmt.Println("Create Error", err)
		return -fuse.EIO
	}
	n, errw := intermediateFile.WriteAt(buff, ofst)
	if errw != nil {
		fmt.Println("Write Error", err)

		return -fuse.EIO
	}
	err = intermediateFile.Close()
	if err != nil {
		fmt.Println("Close Error", err)

		return -fuse.EIO
	}
	ofst += int64(n)

	return n
}

// Flush flushes cached file data.

func (fs *AliYunDriveFS) Flush(path string, fh uint64) int {
	return 0
}

// Release closes an open file.

func (fs *AliYunDriveFS) Release(path string, fh uint64) int {
	parent, name, node := fs.lookupNode(path, nil)

	if node._if == "" {
		fs.closeNode(fh)
		return 0
	}
	node.lock.Lock()
	defer node.lock.Unlock()
	intermediateFile, err := os.OpenFile(node._if, os.O_RDWR, 600)
	fmt.Println(node._if)
	defer intermediateFile.Close()
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			utils.Verbose(utils.VerboseLog, err, name)
		}
	}(intermediateFile.Name())
	stat, err := intermediateFile.Stat()
	if err != nil {
		fs.closeNode(fh)
		return 0
	}
	fileId := aliyun.ContentHandle(intermediateFile, fs.Config.Token, fs.Config.DriveId, parent.fileId, name, uint64(stat.Size()))
	node.stat.Size = stat.Size()
	node.fileId = fileId
	node.parentFileId = parent.fileId
	node._if = ""
	fs.closeNode(fh)
	return 0
}

// Fsync synchronizes file contents.

func (fs *AliYunDriveFS) Fsync(path string, datasync bool, fh uint64) int {
	fmt.Println("fsync")
	return 0

}

/*
// Lock performs a file locking operation.

func (fs *AliYunDriveFS) Lock(path string, cmd int, lock *Lock_t, fh uint64) int {
	return -fuse.ENOSYS
}
*/

// Opendir opens a directory.

func (fs *AliYunDriveFS) Opendir(path string) (int, uint64) {
	return fs.openNode(path, true)
}

// Readdir reads a directory.

func (fs *AliYunDriveFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {

	node := fs.openmap[fh]
	fill(".", &node.stat, 0)
	fill("..", nil, 0)
	for name, chld := range node.chld {
		if !fill(name, &chld.stat, 0) {
			break
		}
	}
	return 0
}

// Releasedir closes an open directory.

func (fs *AliYunDriveFS) Releasedir(path string, fh uint64) int {
	fs.closeNode(fh)
	return 0
}

// Fsyncdir synchronizes directory contents.

func (fs *AliYunDriveFS) Fsyncdir(path string, datasync bool, fh uint64) int {
	fmt.Println("fsysndir", path)
	return 0
}

// Setxattr sets extended attributes.

func (fs *AliYunDriveFS) Setxattr(path string, name string, value []byte, flags int) int {
	fmt.Println("setxattr", path, name)
	_, _, node := fs.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP
	}
	if fuse.XATTR_CREATE == flags {
		if _, ok := node.xatr[name]; ok {
			return -fuse.EEXIST
		}
	} else if fuse.XATTR_REPLACE == flags {
		if _, ok := node.xatr[name]; !ok {
			return -fuse.ENOATTR
		}
	}
	xatr := make([]byte, len(value))
	copy(xatr, value)
	if nil == node.xatr {
		node.xatr = map[string][]byte{}
	}
	node.xatr[name] = xatr
	return 0
}

// Getxattr gets extended attributes.

func (fs *AliYunDriveFS) Getxattr(path string, name string) (int, []byte) {
	fmt.Println("getxattr", path, name)
	_, _, node := fs.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT, nil
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP, nil
	}
	xatr, ok := node.xatr[name]
	if !ok {
		return -fuse.ENOATTR, nil
	}
	return 0, xatr
}

// Removexattr removes extended attributes.

func (fs *AliYunDriveFS) Removexattr(path string, name string) int {
	fmt.Println("removexattr", path, name)
	_, _, node := fs.lookupNode(path, nil)
	if nil == node {
		return -fuse.ENOENT
	}
	if "com.apple.ResourceFork" == name {
		return -fuse.ENOTSUP
	}
	if _, ok := node.xatr[name]; !ok {
		return -fuse.ENOATTR
	}
	delete(node.xatr, name)
	return 0
}

// Listxattr lists extended attributes.

func (fs *AliYunDriveFS) Listxattr(path string, fill func(name string) bool) int {
	return 0
}
