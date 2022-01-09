# goaldfuse
[![made-with-Go](https://img.shields.io/badge/Made%20with-Go-1f425f.svg)](http://golang.org)
[![Github tag](https://badgen.net/github/tag/topcheer/goaldfuse)](https://github.com/topcheer/goaldfuse/tags/)

[![Generic badge](https://img.shields.io/badge/Linux-Ok-green.svg)](https://shields.io/)
[![Generic badge](https://img.shields.io/badge/MacOS-Ok-green.svg)](https://shields.io/)
[![Generic badge](https://img.shields.io/badge/Windows-OK-green.svg)](https://shields.io/)

Mount Aliyun Drive as FUSE Drive

Linux & MacOS

`./goaldfuse -mp MountPoint -rt REFRESH_TOKEN`

* Default Mount Point /tmp/RT

Window

`touch .refresh_token`

`echo YOUR_REFRESH_TOKEN .refresh_token>`

`goalidfuse start`

* Default Mount to G: , so please make it available before mounting



[![GitHub license](https://badgen.net/github/license/topcheer/goaldfuse)](https://github.com/topcheer/goaldfuse/blob/master/LICENSE)

Some code from [go-aliyundrive-webdav](https://github.com/LinkLeong/go-aliyundrive-webdav)