//go:build windows
// +build windows

package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/topcheer/daemon"
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

var Version = "v1.1.19"

type FsHost struct {
	//host *fuse.FileSystemHost
	command *cobra.Command
}

func (f FsHost) PidSavePath() string {
	return "./"
}

func (f FsHost) Name() string {
	vn, _ := f.command.PersistentFlags().GetString("volume-name")
	return "goaldfuse" + vn
}

func (f FsHost) Start() {
	var rt string
	var mp string
	var vm string
	var err error
	rt, err = f.command.PersistentFlags().GetString("refresh-token")
	if err != nil {
		rt = ""
	}
	mp, err = f.command.PersistentFlags().GetString("mount-point")
	if err != nil {
		mp = "G:"
	}
	vm, err = f.command.PersistentFlags().GetString("volume-name")
	if err != nil {
		vm = "阿里云盘"
	}
	MountMe(rt, mp, vm)

}

func (f FsHost) Stop() error {
	return nil
}

func (f FsHost) Restart() error {
	return nil
}

func MountMe(rt string, mp string, volname string) {
	//func main() {

	if len(rt) == 0 {
		rt1, ok := ioutil.ReadFile(".refresh_token")
		if ok != nil {
			fmt.Println("Refresh token required,use touch .refresh_token;echo YOUR_REFRESH_TOKE > .refresh_token")
			return
		}
		rt = string(rt1)
	}

	rr := aliyun.RefreshToken(rt)
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
	if runtime.GOOS == "windows" && len(mp) == 0 {
		mp = "G:"
	}
	options := []string{"-o", "volname=" + volname, "-o", "uid=0", "-o", "gid=0"}
	afs.SetCapReaddirPlus(true)
	afs.SetCapCaseInsensitive(true)

	afs.Mount(mp, options)
	defer func(dir string) {
		afs.Unmount()

	}(mp)
	if err != nil {
		return
	}

	if runtime.GOOS != "windows" {
		err = os.RemoveAll(mp)
	}
	if err != nil {
		return
	}
}

func main() {

	var refreshToken string
	var _mp string
	var volname string
	//var version *bool
	command := daemon.GetCommand()
	command.TraverseChildren = true
	//command.Flags().BoolVarP(version, "version", "v", true, "Print version and exit")
	command.PersistentFlags().StringVarP(&refreshToken, "refresh-token", "r", "", "refresh_token")
	command.PersistentFlags().StringVarP(&_mp, "mount-point", "m", "G:", "mount-point，use any available drive")
	command.PersistentFlags().StringVarP(&volname, "volume-name", "v", "阿里云盘", "volume-name，default to 阿里云盘")

	host := &FsHost{command: command}
	errp := command.ParseFlags(os.Args)
	if errp != nil {
		return
	}
	vn, _ := command.PersistentFlags().GetString("volume-name")
	out, _ := os.OpenFile("./goaldfuse"+vn+".log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	err, _ := os.OpenFile("./goaldfuse_err"+vn+".log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)

	proc := daemon.NewProcess(host).SetPipeline(nil, out, err)
	proc.On(os.Kill, func() {
		fmt.Println("kill received")
		return
	})
	daemon.Register(proc)

	// Start
	if rs := daemon.Run(); rs != nil {
		log.Fatalln(rs)
	}

}
