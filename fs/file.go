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

package fs

import (
	"github.com/jacobsa/fuse"
	"goaldfuse/aliyun"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type FileHandle struct {
	inode *Inode
	key   string

	mpuName   *string
	dirty     bool
	writeInit sync.Once
	mpuWG     sync.WaitGroup

	mu              sync.Mutex
	nextWriteOffset int64
	lastPartId      uint32
	lastWriteError  error
	poolHandle      *BufferPool
	buf             *MBuf

	// read
	reader        io.ReadCloser
	readBufOffset int64

	keepPageCache  bool // the same value we returned to OpenFile
	intermediaFile string
}

// NewFileHandle returns a new file handle for the given `inode` triggered by fuse
// operation with the given `opMetadata`
func NewFileHandle(inode *Inode) *FileHandle {
	fh := &FileHandle{inode: inode}
	return fh
}

func (fh *FileHandle) WriteFile(offset int64, data []byte) (err error) {
	fh.inode.logFuse("WriteFile", offset, len(data))

	fh.mu.Lock()
	defer fh.mu.Unlock()
	var tempFilename = fh.inode.Name + strconv.FormatUint(uint64(fh.inode.Id), 10)

	intermediateFile, err := os.OpenFile(tempFilename, os.O_RDWR|os.O_CREATE, 0666)

	if fh.intermediaFile == "" {
		fh.intermediaFile = tempFilename
	}
	if err != nil {
		return err
	}
	_, err = intermediateFile.Seek(offset, 0)
	if err != nil {
		return err
	}
	_, err = intermediateFile.Write(data)
	if err != nil {
		return err
	}
	err = intermediateFile.Close()
	if err != nil {
		return err
	}
	fh.nextWriteOffset = fh.nextWriteOffset + int64(len(data))
	fh.inode.Attributes.Size = uint64(fh.nextWriteOffset)
	fh.inode.Attributes.Mtime = time.Now()

	return
}

func (fh *FileHandle) ReadFile(offset int64, buf []byte) (bytesRead int, err error) {
	fh.inode.logFuse("ReadFile", offset, len(buf))
	defer func() {
		fh.inode.logFuse("< ReadFile", bytesRead, err)

		if err != nil {
			if err == io.EOF {
				err = nil
			}
		}
	}()

	fh.mu.Lock()
	defer fh.mu.Unlock()

	noWant := len(buf)
	var noRead int

	for bytesRead < noWant && err == nil {
		noRead, err = fh.readFile(offset+int64(bytesRead), buf[bytesRead:])
		if noRead > 0 {
			bytesRead += noRead
		}
	}

	return
}

func (fh *FileHandle) readFile(offset int64, buf []byte) (bytesRead int, err error) {
	defer func() {
		if bytesRead > 0 {
			fh.readBufOffset += int64(bytesRead)
		}

		fh.inode.logFuse("< readFile", bytesRead, err)
	}()

	if uint64(offset) >= fh.inode.Attributes.Size {
		// nothing to read
		if fh.inode.Invalid {
			err = fuse.ENOENT
		} else if fh.inode.KnownSize == nil {
			err = io.EOF
		} else {
			err = io.EOF
		}
		return
	}

	fs := fh.inode.fs

	if fh.poolHandle == nil {
		fh.poolHandle = fs.bufferPool
	}

	if fh.readBufOffset != offset {
		// XXX out of order read, maybe disable prefetching
		fh.inode.logFuse("out of order read", offset, fh.readBufOffset)

		fh.readBufOffset = offset
		if fh.reader != nil {
			err := fh.reader.Close()
			if err != nil {
				return 0, err
			}
			fh.reader = nil
		}
	}

	bytesRead, err = fh.readFromStream(offset, buf)

	return
}

func (fh *FileHandle) Release() {

	if fh.reader != nil {
		err := fh.reader.Close()
		if err != nil {
			return
		}
	}

	// write buffers

	fh.inode.mu.Lock()
	defer fh.inode.mu.Unlock()

	if atomic.AddUint64(&fh.inode.fileHandles, 1) == 1 {
		panic(fh.inode.fileHandles)
	}
}

func (fh *FileHandle) readFromStream(offset int64, buf []byte) (bytesRead int, err error) {
	defer func() {
		if fh.inode.fs.flags.DebugFuse {
			fh.inode.logFuse("< readFromStream", bytesRead)
		}
	}()
	config := fh.inode.fs.Config
	if uint64(offset) >= fh.inode.Attributes.Size {
		// nothing to read
		return
	}

	if fh.reader == nil {
		downloadUrl := aliyun.GetDownloadUrl(config.Token, config.DriveId, fh.inode.FileId)
		var rangeString string
		if uint64(offset+int64(len(buf))+1) >= fh.inode.Attributes.Size {
			rangeString = "bytes=" + strconv.FormatUint(uint64(offset), 10) + "-"
		} else {
			rangeString = "bytes=" + strconv.FormatUint(uint64(offset), 10) + "-" + strconv.FormatUint(uint64(offset+int64(len(buf))), 10)
		}
		for i := 0; i < 5; i++ {
			resp := aliyun.GetFile(downloadUrl, config.Token, rangeString)
			if resp == nil {
				time.Sleep(5 * time.Second)
				continue
			} else {
				fh.reader = resp.Body
				break
			}
		}

	}

	bytesRead, err = fh.reader.Read(buf)
	if err != nil {
		if err != io.EOF {
			fh.inode.logFuse("< readFromStream error", bytesRead, err)
		}
		// always retry error on read
		err := fh.reader.Close()
		if err != nil {
			return 0, err
		}
		fh.reader = nil
		err = nil
	}

	return
}

func (fh *FileHandle) FlushFile() (err error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	intermediateFile, err := os.OpenFile(fh.intermediaFile, os.O_RDWR, 600)
	fs := fh.inode.fs
	stat, err := intermediateFile.Stat()
	if err != nil {
		return err
	}
	aliyun.ContentHandle(intermediateFile, fs.Config.Token, fs.Config.DriveId, fh.inode.Parent.FileId, fh.inode.Name, uint64(stat.Size()))

	return
}
