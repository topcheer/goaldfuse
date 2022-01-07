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
	"github.com/shirou/gopsutil/process"
	"time"
)

var TIME_MAX = time.Unix(1<<63-62135596801, 999999999)

func MaxUInt64(a, b uint64) uint64 {
	if a > b {
		return a
	} else {
		return b
	}
}

func TryUnmount(mountPoint string) (err error) {
	for i := 0; i < 20; i++ {
		err = fuse.Unmount(mountPoint)
		if err != nil {
			time.Sleep(time.Second)
		} else {
			break
		}
	}
	return
}

// GetTgid returns the tgid for the given pid.
func GetTgid(pid uint32) (tgid *int32, err error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil, err
	}
	tgidVal, err := p.Tgid()
	if err != nil {
		return nil, err
	}
	return &tgidVal, nil
}
