//go:build windows
// +build windows

package main

import (
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/medivh-jay/daemon"
	"goaldfuse/aliyun"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/fs_windows"
	"goaldfuse/utils"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"runtime"
	"time"
)

var Version = "v1.1.1"

type FsHost struct {
	//host *fuse.FileSystemHost
}

func (f FsHost) PidSavePath() string {
	return "./"
}

func (f FsHost) Name() string {
	return "goaldfuse"
}

func (f FsHost) Start() {
	MountMe()

}

func (f FsHost) Stop() error {
	return nil
}

func (f FsHost) Restart() error {
	return nil
}

func MountMe() {
	//func main() {
	var refreshToken *string
	var mp *string
	var version *bool

	refreshToken = flag.String("rt", "", "refresh_token")
	mp = flag.String("mp", "G:", "mount_point，use any available drive")
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
	utils.AccessToken = rr.AccessToken
	utils.DriveId = rr.DefaultDriveId
	cache.Init()
	afs := fs_windows.NewAliYunDriveFSHost(*config)

	utils.VerboseLog = true
	mountPoint := *mp
	if len(*mp) == 0 {
		mountPoint = "/tmp/" + uuid.New().String()
	}
	if runtime.GOOS == "windows" && len(*mp) == 0 {
		mountPoint = "G:"
	}
	options := []string{"-o", "volname=阿里云盘", "-o", "uid=0", "-o", "gid=0"}
	afs.SetCapReaddirPlus(true)
	afs.SetCapCaseInsensitive(true)

	afs.Mount(mountPoint, options)
	defer func(dir string) {
		afs.Unmount()

	}(mountPoint)
	if err != nil {
		return
	}

	if runtime.GOOS != "windows" {
		err = os.RemoveAll(mountPoint)
	}
	if err != nil {
		return
	}
}

func main() {
	out, _ := os.OpenFile("./goaldfuse.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	err, _ := os.OpenFile("./goaldfuse_err.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)

	// Use daemon.NewProcess to make your worker have signal monitoring, restart listening, and turn off listening, SetPipeline it's not necessary.
	proc := daemon.NewProcess(new(FsHost)).SetPipeline(nil, out, err)
	// Start
	if rs := daemon.Run(); rs != nil {
		log.Fatalln(rs)
	}

}
