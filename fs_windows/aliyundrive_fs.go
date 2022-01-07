package fs_windows

import (
	"github.com/billziss-gh/cgofuse/fuse"
	"goaldfuse/aliyun/model"
)

func NewAliYunDriveFSHost(config model.Config) *fuse.FileSystemHost {
	fs := &AliYunDriveFS{Config: config}
	host := fuse.NewFileSystemHost(fs)

	return host
}

type AliYunDriveFS struct {
	fuse.FileSystemInterface
	//Ali API Config
	Config model.Config
}

// Statfs gets file system statistics.

func (*AliYunDriveFS) Statfs(path string, stat *fuse.Statfs_t) int {
	return -fuse.ENOSYS
}

// Mknod creates a file node.

func (*AliYunDriveFS) Mknod(path string, mode uint32, dev uint64) int {
	return -fuse.ENOSYS
}

// Mkdir creates a directory.

func (*AliYunDriveFS) Mkdir(path string, mode uint32) int {
	return -fuse.ENOSYS
}

// Unlink removes a file.

func (*AliYunDriveFS) Unlink(path string) int {
	return -fuse.ENOSYS
}

// Rmdir removes a directory.

func (*AliYunDriveFS) Rmdir(path string) int {
	return -fuse.ENOSYS
}

// Link creates a hard link to a file.

func (*AliYunDriveFS) Link(oldpath string, newpath string) int {
	return -fuse.ENOSYS
}

// Symlink creates a symbolic link.

func (*AliYunDriveFS) Symlink(target string, newpath string) int {
	return -fuse.ENOSYS
}

// Readlink reads the target of a symbolic link.

func (*AliYunDriveFS) Readlink(path string) (int, string) {
	return -fuse.ENOSYS, ""
}

// Rename renames a file.

func (*AliYunDriveFS) Rename(oldpath string, newpath string) int {
	return -fuse.ENOSYS
}

// Chmod changes the permission bits of a file.

func (*AliYunDriveFS) Chmod(path string, mode uint32) int {
	return -fuse.ENOSYS
}

// Chown changes the owner and group of a file.

func (*AliYunDriveFS) Chown(path string, uid uint32, gid uint32) int {
	return -fuse.ENOSYS
}

// Utimens changes the access and modification times of a file.

func (*AliYunDriveFS) Utimens(path string, tmsp []fuse.Timespec) int {
	return -fuse.ENOSYS
}

// Access checks file access permissions.

func (*AliYunDriveFS) Access(path string, mask uint32) int {
	return -fuse.ENOSYS
}

// Create creates and opens a file.
// The flags are a combination of the fuse.O_* constants.

func (*AliYunDriveFS) Create(path string, flags int, mode uint32) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

// Open opens a file.
// The flags are a combination of the fuse.O_* constants.

func (*AliYunDriveFS) Open(path string, flags int) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

// Getattr gets file attributes.

func (*AliYunDriveFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	return -fuse.ENOSYS
}

// Truncate changes the size of a file.

func (*AliYunDriveFS) Truncate(path string, size int64, fh uint64) int {
	return -fuse.ENOSYS
}

// Read reads data from a file.

func (*AliYunDriveFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	return -fuse.ENOSYS
}

// Write writes data to a file.

func (*AliYunDriveFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	return -fuse.ENOSYS
}

// Flush flushes cached file data.

func (*AliYunDriveFS) Flush(path string, fh uint64) int {
	return -fuse.ENOSYS
}

// Release closes an open file.

func (*AliYunDriveFS) Release(path string, fh uint64) int {
	return -fuse.ENOSYS
}

// Fsync synchronizes file contents.

func (*AliYunDriveFS) Fsync(path string, datasync bool, fh uint64) int {
	return -fuse.ENOSYS
}

/*
// Lock performs a file locking operation.

func (*AliYunDriveFS) Lock(path string, cmd int, lock *Lock_t, fh uint64) int {
	return -fuse.ENOSYS
}
*/

// Opendir opens a directory.

func (*AliYunDriveFS) Opendir(path string) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

// Readdir reads a directory.

func (*AliYunDriveFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	return -fuse.ENOSYS
}

// Releasedir closes an open directory.

func (*AliYunDriveFS) Releasedir(path string, fh uint64) int {
	return -fuse.ENOSYS
}

// Fsyncdir synchronizes directory contents.

func (*AliYunDriveFS) Fsyncdir(path string, datasync bool, fh uint64) int {
	return -fuse.ENOSYS
}

// Setxattr sets extended attributes.

func (*AliYunDriveFS) Setxattr(path string, name string, value []byte, flags int) int {
	return -fuse.ENOSYS
}

// Getxattr gets extended attributes.

func (*AliYunDriveFS) Getxattr(path string, name string) (int, []byte) {
	return -fuse.ENOSYS, nil
}

// Removexattr removes extended attributes.

func (*AliYunDriveFS) Removexattr(path string, name string) int {
	return -fuse.ENOSYS
}

// Listxattr lists extended attributes.

func (*AliYunDriveFS) Listxattr(path string, fill func(name string) bool) int {
	return -fuse.ENOSYS
}
