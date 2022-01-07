//go:build windows
// +build windows

package main

import (
	"flag"
	"fmt"
	"github.com/google/uuid"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/fs_windows"
	"goaldfuse/utils"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"time"
)

var Version = "v1.0.11"

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
	afs := fs_windows.NewAliYunDriveFSHost(*config)
	cache.Init()

	utils.VerboseLog = true
	mountPoint := *mp
	if len(*mp) == 0 {
		mountPoint = "/tmp/" + uuid.New().String()
	}
	if runtime.GOOS == "windows" && len(*mp) == 0 {
		mountPoint = "c:\\tmp\\" + uuid.New().String()
	}
	if runtime.GOOS != "windows" {
		err = os.Mkdir(mountPoint, os.FileMode(0755))
		if err != nil {
			fmt.Println("Failed to create mount point", mountPoint, err)
			return
		}
	}

	afs.Mount(mountPoint, nil)
	defer func(dir string) {
		afs.Unmount()

	}(mountPoint)
	if err != nil {
		return
	}

	succeed := afs.Unmount()
	if !succeed {
		fmt.Println("Failed to Unmount", mountPoint)
	}
	if runtime.GOOS != "windows" {
		err = os.RemoveAll(mountPoint)
	}
	if err != nil {
		return
	}
}
