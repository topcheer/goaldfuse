//go:build !windows
// +build !windows

package main

import (
	"context"
	"fmt"
	"github.com/jacobsa/fuse"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/fs"
	"goaldfuse/utils"
	"os"
	"reflect"
	"time"
)

func main() {
	rr := aliyun.RefreshToken("")
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
	os.Mkdir("/tmp/ali2", os.FileMode(777))
	mfs, err := fuse.Mount("/tmp/ali2", afs, mountConfig)
	defer fuse.Unmount("/tmp/ali2")
	if err != nil {
		return
	}
	errs := mfs.Join(context.Background())
	if err != nil {
		fmt.Println(errs)
	}
	os.RemoveAll("/tmp/ali2")
}
