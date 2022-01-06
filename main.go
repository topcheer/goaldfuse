//go:build !windows
// +build !windows

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/jacobsa/fuse"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/fs"
	"goaldfuse/utils"
	"io/ioutil"
	"os"
	"reflect"
	"time"
)

func main() {
	var refreshToken *string
	var mp *string

	refreshToken = flag.String("rt", "", "refresh_token")
	mp = flag.String("mp", "", "mount_point，will create if not exist")
	flag.Parse()

	rtoken := *refreshToken
	if len(*refreshToken) == 0 {
		rt, ok := ioutil.ReadFile(".refresh_token")
		if ok != nil {
			panic("Refresh token required,use touch .refresh_token;echo YOUR_REFRESH_TOKE > .refresh_token")
		}
		rtoken = string(rt)
	} else {
		rtoken = *refreshToken
	}

	rr := aliyun.RefreshToken(rtoken)
	if reflect.DeepEqual(rr, model.RefreshTokenModel{}) {
		panic("can't get token")
	}
	config := &model.Config{
		DriveId:      rr.DefaultDriveId,
		Token:        rr.AccessToken,
		RefreshToken: rr.RefreshToken,
		ExpireTime:   time.Now().Unix() + rr.ExpiresIn,
	}
	afs, _ := fs.NewAliYunDriveFsServer(*config)
	cache.Init()
	mountConfig := &fuse.MountConfig{FSName: "AliYunDrive",
		ReadOnly:           false,
		VolumeName:         "AliYunDrive",
		EnableVnodeCaching: false,
		//ErrorLogger:        log.New(os.Stderr, "FuseError: ", log.LstdFlags),
		//DebugLogger:        log.New(os.Stdout, "FuseDebug: ", log.LstdFlags),
	}
	utils.VerboseLog = true
	mountPoint := *mp
	if len(*mp) == 0 {
		mountPoint = "/tmp/" + uuid.New().String()
	}

	os.Mkdir(mountPoint, os.FileMode(777))
	mfs, err := fuse.Mount(mountPoint, afs, mountConfig)
	defer func(dir string) {
		err := fuse.Unmount(dir)
		if err != nil {
			return
		}
	}(mountPoint)
	if err != nil {
		return
	}
	errs := mfs.Join(context.Background())
	if err != nil {
		fmt.Println(errs)
	}
	err = fuse.Unmount(mountPoint)
	if err != nil {
		fmt.Println("Failed to Unmount", mountPoint)
	}
	err = os.RemoveAll(mountPoint)
	if err != nil {
		return
	}
}
