package fs_windows

import (
	"fmt"
	"github.com/billziss-gh/cgofuse/fuse"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/model"
	"strconv"
)

func NewAliYunDriveFSHost(config model.Config) *fuse.FileSystemHost {
	fs := &AliYunDriveFS{Config: config}
	host := fuse.NewFileSystemHost(fs)
	fs.Host = host
	return host
}

type AliYunDriveFS struct {
	fuse.FileSystemInterface
	//Ali API Config
	Config model.Config
	Host   *fuse.FileSystemHost
}

var TOTAL uint64
var USED uint64

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
	fmt.Println("stat", path, stat)

	if TOTAL == 0 && USED == 0 {
		total, used := aliyun.GetBoxSize(fs.Config.Token)
		TOTAL, _ = strconv.ParseUint(total, 10, 64)
		USED, _ = strconv.ParseUint(used, 10, 64)
	}

	stat.Bsize = 4096

	const INODES = 1 * 1000 * 1000 * 1000 // 1 billion
	stat.Blocks = TOTAL / uint64(stat.Bsize)
	stat.Bfree = (TOTAL - USED) / uint64(stat.Bsize)
	stat.Bavail = (TOTAL - USED) / uint64(stat.Bsize)
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
	return -fuse.ENOSYS
}

// Mkdir creates a directory.

func (fs *AliYunDriveFS) Mkdir(path string, mode uint32) int {
	return -fuse.ENOSYS
}

// Unlink removes a file.

func (fs *AliYunDriveFS) Unlink(path string) int {
	return -fuse.ENOSYS
}

// Rmdir removes a directory.

func (fs *AliYunDriveFS) Rmdir(path string) int {
	return -fuse.ENOSYS
}

// Link creates a hard link to a file.

func (fs *AliYunDriveFS) Link(oldpath string, newpath string) int {
	return -fuse.ENOSYS
}

// Symlink creates a symbolic link.

func (fs *AliYunDriveFS) Symlink(target string, newpath string) int {
	return -fuse.ENOSYS
}

// Readlink reads the target of a symbolic link.

func (fs *AliYunDriveFS) Readlink(path string) (int, string) {
	return -fuse.ENOSYS, ""
}

// Rename renames a file.

func (fs *AliYunDriveFS) Rename(oldpath string, newpath string) int {
	return -fuse.ENOSYS
}

// Chmod changes the permission bits of a file.

func (fs *AliYunDriveFS) Chmod(path string, mode uint32) int {
	return -fuse.ENOSYS
}

// Chown changes the owner and group of a file.

func (fs *AliYunDriveFS) Chown(path string, uid uint32, gid uint32) int {
	return -fuse.ENOSYS
}

// Utimens changes the access and modification times of a file.

func (fs *AliYunDriveFS) Utimens(path string, tmsp []fuse.Timespec) int {
	return -fuse.ENOSYS
}

// Access checks file access permissions.

func (fs *AliYunDriveFS) Access(path string, mask uint32) int {
	fmt.Println("Access", path)
	return -fuse.ENOSYS
}

// Create creates and opens a file.
// The flags are a combination of the fuse.O_* constants.

func (fs *AliYunDriveFS) Create(path string, flags int, mode uint32) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

// Open opens a file.
// The flags are a combination of the fuse.O_* constants.

func (fs *AliYunDriveFS) Open(path string, flags int) (int, uint64) {
	fmt.Println("Open", path)
	return -fuse.ENOSYS, ^uint64(0)
}

// Getattr gets file attributes.

func (fs *AliYunDriveFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	fmt.Println("getattr", path)
	return 0
}

// Truncate changes the size of a file.

func (fs *AliYunDriveFS) Truncate(path string, size int64, fh uint64) int {
	return 0
}

// Read reads data from a file.

func (fs *AliYunDriveFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fmt.Println("read", path)
	return 0
}

// Write writes data to a file.

func (fs *AliYunDriveFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	return 0
}

// Flush flushes cached file data.

func (fs *AliYunDriveFS) Flush(path string, fh uint64) int {
	return 0
}

// Release closes an open file.

func (fs *AliYunDriveFS) Release(path string, fh uint64) int {
	fmt.Println("release", path)
	return 0
}

// Fsync synchronizes file contents.

func (fs *AliYunDriveFS) Fsync(path string, datasync bool, fh uint64) int {
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
	fmt.Println("opendir", path)
	return 0, ^uint64(0)
}

// Readdir reads a directory.

func (fs *AliYunDriveFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fmt.Println("readdir", path)
	return 0
}

// Releasedir closes an open directory.

func (fs *AliYunDriveFS) Releasedir(path string, fh uint64) int {
	fmt.Println("releasedir", path)
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
	return 0
}

// Getxattr gets extended attributes.

func (fs *AliYunDriveFS) Getxattr(path string, name string) (int, []byte) {
	fmt.Println("getxattr", path, name)
	return 0, nil
}

// Removexattr removes extended attributes.

func (fs *AliYunDriveFS) Removexattr(path string, name string) int {
	fmt.Println("removexattr", path, name)
	return 0
}

// Listxattr lists extended attributes.

func (fs *AliYunDriveFS) Listxattr(path string, fill func(name string) bool) int {
	fmt.Println("listxattr", path, fill("xx"))
	return 0
}
