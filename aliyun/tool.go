package aliyun

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"github.com/tidwall/gjson"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/aliyun/net"
	"goaldfuse/utils"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

//处理内容
func ContentHandle(intermediateFile *os.File, token string, driveId string, parentId string, fileName string, size uint64) string {
	//需要判断参数里面的有效期
	//默认截取长度10485760
	//const DEFAULT int64 = 10485760
	const DEFAULT int64 = 10485760
	var count float64 = 1

	if len(parentId) == 0 {
		parentId = "root"
	}
	if size > 0 {
		count = math.Ceil(float64(size) / float64(DEFAULT))
	} else {
		//空文件处理
		sha1_0 := "DA39A3EE5E6B4B0D3255BFEF95601890AFD80709"
		_, _, fileId, _ := UpdateFileFile(token, driveId, fileName, parentId, "0", 1, sha1_0, "", true)
		if fileId != "" {
			utils.Verbose(utils.VerboseLog, "0⃣️  Created zero byte file", fileName)
			if va, ok := cache.GoCache.Get(parentId); ok {
				l := va.(model.FileListModel)
				l.Items = append(l.Items, GetFileDetail(token, driveId, fileId))
				cache.GoCache.SetDefault(parentId, l)
			}

			return fileId
		} else {
			utils.Verbose(utils.VerboseLog, "❌  Unable to create zero byte file", fileName)
			return ""
		}
	}

	//proof 偏移量
	var offset int64 = 0
	//proof内容base64
	var proof string = ""
	//是否闪传
	var flashUpload bool = false
	//status code
	var code int
	var uploadUrl []gjson.Result
	var uploadId string
	var uploadFileId string

	defer func(create *os.File) {
		err := create.Close()
		if err != nil {
			utils.Verbose(utils.VerboseLog, err)
		}
	}(intermediateFile)

	//大于15K小于25G的才开启闪传
	if size > 1024*15 && size <= 1024*1024*1024*25 {
		preHashDataBytes := make([]byte, 1024)
		_, err := intermediateFile.ReadAt(preHashDataBytes, 0)
		if err != nil {
			utils.Verbose(utils.VerboseLog, "❌  error reading file", intermediateFile.Name(), err, fileName)
			return ""
		}
		h := sha1.New()
		h.Write(preHashDataBytes)
		//检查是否可以极速上传，逻辑如下
		//取文件的前1K字节，做SHA1摘要，调用创建文件接口，pre_hash参数为SHA1摘要，如果返回409，则这个文件可以极速上传
		preHashRequest := `{"drive_id":"` + driveId + `","parent_file_id":"` + parentId + `","name":"` + fileName + `","type":"file","check_name_mode":"overwrite","size":` + strconv.FormatUint(size, 10) + `,"pre_hash":"` + hex.EncodeToString(h.Sum(nil)) + `","proof_version":"v1"}`
		_, code = net.PostExpectStatus(model.APIFILEUPLOAD, token, []byte(preHashRequest))
		if code == 409 {
			md := md5.New()
			tokenBytes := []byte(token)
			md.Write(tokenBytes)
			tokenMd5 := hex.EncodeToString(md.Sum(nil))
			first16 := tokenMd5[:16]
			f, err := strconv.ParseUint(first16, 16, 64)
			if err != nil {
				utils.Verbose(utils.VerboseLog, err)
			}
			offset = int64(f % size)
			end := math.Min(float64(offset+8), float64(size))
			off := make([]byte, int64(end)-offset)
			_, errS := intermediateFile.Seek(0, 0)
			if errS != nil {
				utils.Verbose(utils.VerboseLog, "❌  error seek file", intermediateFile.Name(), err, fileName)
				return ""
			}
			_, offerr := intermediateFile.ReadAt(off, offset)
			if offerr != nil {
				utils.Verbose(utils.VerboseLog, "❌  Can't calculate proof", offerr, fileName)
				return ""
			}
			proof = utils.GetProof(off)
			flashUpload = true
		}
		_, seekError := intermediateFile.Seek(0, 0)
		if seekError != nil {
			utils.Verbose(utils.VerboseLog, "❌  seek error ", seekError, fileName, intermediateFile.Name())
			return ""
		}
		h2 := sha1.New()
		_, sha1Error := io.Copy(h2, intermediateFile)
		if sha1Error != nil {
			utils.Verbose(utils.VerboseLog, "❌  Error calculate SHA1", sha1Error, fileName, intermediateFile.Name(), size)
			return ""
		}
		uploadUrl, uploadId, uploadFileId, flashUpload = UpdateFileFile(token, driveId, fileName, parentId, strconv.FormatUint(size, 10), int(count), strings.ToUpper(hex.EncodeToString(h2.Sum(nil))), proof, flashUpload)
		if flashUpload && (uploadFileId != "") {
			utils.Verbose(utils.VerboseLog, "⚡️⚡️  Rapid Upload ", fileName, size)
			//UploadFileComplete(token, driveId, uploadId, uploadFileId, parentId)
			if va, ok := cache.GoCache.Get(parentId); ok {
				l := va.(model.FileListModel)
				l.Items = append(l.Items, GetFileDetail(token, driveId, uploadFileId))
				cache.GoCache.SetDefault(parentId, l)
			}
			return uploadFileId
		}
	} else {
		uploadUrl, uploadId, uploadFileId, flashUpload = UpdateFileFile(token, driveId, fileName, parentId, strconv.FormatUint(size, 10), int(count), "", "", false)
	}

	if len(uploadUrl) == 0 {
		utils.Verbose(utils.VerboseLog, "❌ ❌  Empty UploadUrl", fileName, size, uploadId, uploadFileId)
		return ""
	}
	var bg time.Time = time.Now()
	stat, err := intermediateFile.Stat()
	if err != nil {
		utils.Verbose(utils.VerboseLog, "❌ can't stat file", err, fileName)
		return ""
	}

	utils.Verbose(utils.VerboseLog, "📢  Normal upload ", fileName, uploadId, size, stat.Size())
	_, e1 := intermediateFile.Seek(0, 0)
	if e1 != nil {
		utils.Verbose(utils.VerboseLog, "❌ ❌ Seek err", e1, fileName)
		return ""
	}
	for i := 0; i < int(count); i++ {
		utils.Verbose(utils.VerboseLog, "📢  Uploading part:", i+1, "Total:", count, fileName)
		pstart := time.Now()
		var dataByte []byte
		if int(count) == 1 {
			dataByte = make([]byte, size)
		} else if i == int(count)-1 {
			dataByte = make([]byte, int64(size)-int64(i)*DEFAULT)
		} else {
			dataByte = make([]byte, DEFAULT)
		}
		_, err := io.ReadFull(intermediateFile, dataByte)
		if err != nil {
			utils.Verbose(utils.VerboseLog, "❌  error reading from temp file", err, intermediateFile.Name(), fileName, uploadId)
			return ""
		}
		//check if upload url has expired
		uri := uploadUrl[i].Str
		idx := strings.Index(uri, "x-oss-expires=") + len("x-oss-expires=")
		idx2 := strings.Index(uri[idx:], "&")
		exp := uri[idx : idx2+idx]
		expire, _ := strconv.ParseInt(exp, 10, 64)
		if time.Now().UnixMilli()/1000 > expire {
			utils.Verbose(utils.VerboseLog, "⚠️     Now:", time.Now().UnixMilli()/1000)
			utils.Verbose(utils.VerboseLog, "⚠️  Expire:", exp)
			utils.Verbose(utils.VerboseLog, "⚠️  Uploading URL expired, renewing", uploadId, uploadFileId, fileName)
			for i := 0; i < 10; i++ {
				uploadUrl = GetUploadUrls(utils.AccessToken, utils.DriveId, uploadFileId, uploadId, int(count))
				if len(uploadUrl) == int(count) {
					break
				}
				utils.Verbose(utils.VerboseLog, "Retry in 10 seconds")
				time.Sleep(10 * time.Second)
			}

			if len(uploadUrl) == 0 {
				//长时间上传可能之前传入的token已经过期，从全局变量中取
				utils.Verbose(utils.VerboseLog, "❌  Renew Uploading URL failed", fileName, uploadId, uploadFileId, "cancel upload")
				return ""
			} else {
				//utils.Verbose(utils.VerboseLog,"ℹ️  从头再来 💃🤔⬆️‼️ Resetting upload part")
				//i = 0
				utils.Verbose(utils.VerboseLog, "  💻  Renew Upload URL Done, Total Parts", len(uploadUrl))
			}
		}
		if ok := UploadFile(uploadUrl[i].Str, token, dataByte); !ok {
			utils.Verbose(utils.VerboseLog, "❌  Upload part failed ", fileName, "Part#", i+1, " 😜   Cancel upload")
			return ""
		}
		utils.Verbose(utils.VerboseLog, "✅  Done part:", i+1, "Elapsed:", time.Now().Sub(pstart).String(), fileName)

	}
	utils.Verbose(utils.VerboseLog, "⚡ ⚡ ⚡   Done. Elapsed ", time.Now().Sub(bg).String(), fileName, size)
	//长时间上传可能之前传入的token已经过期，从全局变量中取
	UploadFileComplete(utils.AccessToken, utils.DriveId, uploadId, uploadFileId, parentId)
	if va, ok := cache.GoCache.Get(parentId); ok {
		l := va.(model.FileListModel)
		l.Items = append(l.Items, GetFileDetail(token, driveId, uploadFileId))
		cache.GoCache.SetDefault(parentId, l)
	}
	return uploadFileId
}
