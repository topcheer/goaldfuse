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
	"runtime"
	"time"
)

var Version = "v1.0.14"

func main() {
	var refreshToken *string
	var mp *string
	var version *bool

	refreshToken = flag.String("rt", "", "refresh_token")
	mp = flag.String("mp", "", "mount_point，will create if not exist")
	version = flag.Bool("v", false, "Print version and exit")
	flag.Parse()
	if *version {
		fmt.Println(Version)
		return
	}
	rtoken := *refreshToken
	if len(*refreshToken) == 0 {
		rt, ok := ioutil.ReadFile(".refresh_token")
		if ok != nil {
			fmt.Println("Refresh token required,use touch .refresh_token;echo YOUR_REFRESH_TOKE > .refresh_token")
			return
		}
		rtoken = string(rt)
	} else {
		rtoken = *refreshToken
	}

	rr := aliyun.RefreshToken(rtoken)
	if reflect.DeepEqual(rr, model.RefreshTokenModel{}) {
		fmt.Println("Invalid Refresh Token")
		return
	}
	err := ioutil.WriteFile(".refresh_token", []byte(rr.RefreshToken), 0600)
	if err != nil {
		fmt.Println("Can't write token file, token will not be able to persist")
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
	if runtime.GOOS == "windows" {
		mountPoint = "c:\\tmp\\" + uuid.New().String()
	}

	err = os.Mkdir(mountPoint, os.FileMode(0755))
	if err != nil {
		fmt.Println("Failed to create mount point", mountPoint, err)
		return
	}
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
