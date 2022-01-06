package utils

import "fmt"

// VerboseLog 启动时根据命令行参数设置
var VerboseLog bool = false

// AccessToken 全局变量，每次refreshToken都会刷新，可以在长事务中使用
var AccessToken string
var DriveId string

func Verbose(verbose bool, message ...interface{}) {
	if verbose {
		fmt.Println(message)
	}
}
