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

var Version = "v1.1.17"

type FsHost struct {
	mfs *fuse.MountedFileSystem
}

func (f FsHost) PidSavePath() string {
	return "./"
}

func (f FsHost) Name() string {
	return "goaldfuse"
}

func (f FsHost) Start() {
	go func() {
		err := f.mfs.Join(context.Background())
		if err != nil {
			fmt.Println("unable to go into background")
		}
	}()

}

func (f FsHost) Stop() error {
	err := fuse.Unmount(f.mfs.Dir())
	if err != nil {
		fmt.Println("unable to umount,", f.mfs.Dir())
		return err
	}
	err = os.RemoveAll(f.mfs.Dir())
	if err != nil {
		fmt.Println("unable to remove", f.mfs.Dir())
	}
	return err
}

func (f FsHost) Restart() error {
	err := f.Stop()
	if err != nil {
		return err
	}
	f.Start()
	return nil
}

func main() {
	//func MountMe() *fuse.MountedFileSystem {
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

	err = os.Mkdir(mountPoint, 0777)
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
	err = mfs.Join(context.Background())
	if err != nil {
		fmt.Println("unable to go into background")
	}
	err = fuse.Unmount(mfs.Dir())
	if err != nil {
		fmt.Println("unable to umount,", mfs.Dir())
		return
	}
	err = os.RemoveAll(mfs.Dir())
	if err != nil {
		fmt.Println("unable to remove", mfs.Dir())
	}
	//return mfs
	//err = fuse.Unmount(mountPoint)
	//if err != nil {
	//	fmt.Println("Failed to Unmount", mountPoint)
	//}

}

//func main() {
//	out, _ := os.OpenFile("./goaldfuse.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
//	err, _ := os.OpenFile("./goaldfuse_err.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
//
//	// Use daemon.NewProcess to make your worker have signal monitoring, restart listening, and turn off listening, SetPipeline it's not necessary.
//	host := new(FsHost)
//	host.mfs = MountMe()
//	proc := daemon.NewProcess(host).SetPipeline(nil, out, err)
//	proc.On(os.Kill, func() {
//		err := fuse.Unmount(host.mfs.Dir())
//		if err != nil {
//			fmt.Println("failed to umount", host.mfs.Dir())
//		}
//		err = os.RemoveAll(host.mfs.Dir())
//		if err != nil {
//			fmt.Println("unable to remove", host.mfs.Dir())
//		}
//	})
//	// Start
//	if rs := daemon.Run(); rs != nil {
//		log.Fatalln(rs)
//	}
//
//}
