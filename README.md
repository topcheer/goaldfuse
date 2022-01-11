# goaldfuse 
[![made-with-Go](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](http://golang.org)
[![Github tag](https://badgen.net/github/tag/topcheer/goaldfuse)](https://github.com/topcheer/goaldfuse/tags/)

[![Generic badge](https://img.shields.io/badge/Linux-Ok-green.svg)](https://shields.io/)
[![Generic badge](https://img.shields.io/badge/MacOS-Ok-green.svg)](https://shields.io/)
[![Generic badge](https://img.shields.io/badge/Windows-OK-green.svg)](https://shields.io/)

Mount Aliyun Drive as FUSE Drive

# 阿里云盘作为本地硬盘使用 

具体怎么获取refresh_token请参考 https://github.com/messense/aliyundrive-webdav#%E8%8E%B7%E5%8F%96-refresh_token  

Linux & MacOS

`./goaldfuse -mp MountPoint -rt REFRESH_TOKEN`

* Default Mount Point /tmp/RT
* Only tested with MacFUSE on MacOS, other FUSE implementation should also work

Window

`touch .refresh_token`

`echo YOUR_REFRESH_TOKEN > .refresh_token`

`goalidfuse start`

* Default Mount to G: , so please make it available before mounting
* Tested with Winfsp on Windows 10/11 amd64, install before using this utility
* https://github.com/billziss-gh/winfsp/releases/tag/v1.10
* Latest Windows version https://github.com/topcheer/goaldfuse/releases/download/windows_latest/goladfuse-windows-x64.exe
* 特别说明： Windows对于图片，视频，PDF，Office文档等等会预读取65K～256K不等的文件头 ，如果你在移动流量下使用，一定要注意自己的流量
* 建议只在Wifi环境中使用

[![GitHub license](https://badgen.net/github/license/topcheer/goaldfuse)](https://github.com/topcheer/goaldfuse/blob/master/LICENSE)

Credits:
* Some code from [go-aliyundrive-webdav](https://github.com/LinkLeong/go-aliyundrive-webdav)
* Windows GO FUSE @billziss-gh/cgofuse
* Linux/MacOS GO FUSE @jacobsa/fuse