package aliyun

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tidwall/gjson"
	"goaldfuse/aliyun/cache"
	"goaldfuse/aliyun/model"
	"goaldfuse/aliyun/net"
	"goaldfuse/utils"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func GetList(token string, driveId string, parentFileId string, marker ...string) (model.FileListModel, error) {
	return GetListA(token, driveId, parentFileId, false, marker...)
}

func GetListA(token string, driveId string, parentFileId string, folderOnly bool, marker ...string) (model.FileListModel, error) {

	if len(parentFileId) == 0 {
		parentFileId = "root"
	}

	var list model.FileListModel
	if result, ok := cache.GoCache.Get(parentFileId); ok {
		list, ok = result.(model.FileListModel)
		if ok {
			return list, nil
		}
	}

	postData := make(map[string]interface{})
	postData["drive_id"] = driveId
	postData["parent_file_id"] = parentFileId
	postData["limit"] = 200
	postData["all"] = false
	postData["url_expire_sec"] = 1600
	postData["image_thumbnail_process"] = "image/resize,w_400/format,jpeg"
	postData["image_url_process"] = "image/resize,w_1920/format,jpeg"
	postData["video_thumbnail_process"] = "video/snapshot,t_0,f_jpg,ar_auto,w_300"
	postData["fields"] = "*"
	postData["order_by"] = "updated_at"
	postData["order_direction"] = "DESC"
	//add marker post data
	if len(marker) > 0 {
		postData["marker"] = marker[0]
	}
	if folderOnly {
		postData["type"] = "folder"
	}

	data, err := json.Marshal(postData)
	if err != nil {
		utils.Verbose(utils.VerboseLog, "获取列表转义数据失败", err)
		return model.FileListModel{}, err
	}

	for i := 0; i < 5; i++ {
		body := net.Post(model.APILISTURL, token, data)
		e := json.Unmarshal(body, &list)
		if e != nil {
			utils.Verbose(utils.VerboseLog, "❌  GetList Failed", e, string(body), "retry in 5 seconds")
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	if list.NextMarker != "" {
		//utils.Verbose(utils.VerboseLog,"Next Page Marker: " + list.NextMarker)
		var newList, _ = GetListA(token, driveId, parentFileId, folderOnly, list.NextMarker)
		list.Items = append(list.Items, newList.Items...)
		list.NextMarker = newList.NextMarker
	}
	if len(list.Items) > 0 {
		cache.GoCache.SetDefault(parentFileId, list)
		for _, item := range list.Items {
			cache.GoCache.Set("FI_"+item.FileId, item, -1)
		}
	}
	return list, nil
}

func GetFilePath(token string, driveId string, parentFileId string, fileId string, typeStr string) (string, error) {

	if len(parentFileId) == 0 {
		parentFileId = "root"
	}
	path := "/"
	var list model.ListFilePath
	if result, ok := cache.GoCache.Get(parentFileId + "path"); ok {
		path, ok = result.(string)
		if ok {
			return path, nil
		}
	}

	postData := make(map[string]interface{})
	postData["drive_id"] = driveId
	postData["file_id"] = fileId

	data, err := json.Marshal(postData)
	if err != nil {
		utils.Verbose(utils.VerboseLog, "获取列表转义数据失败", err)
		return "/", err
	}

	body := net.Post(model.APIFILEPATH, token, data)

	e := json.Unmarshal(body, &list)
	if e != nil {
		utils.Verbose(utils.VerboseLog, "❌   GetFilePath Failed", e, string(body))
	}
	minNum := 0
	if typeStr == "folder" {
		minNum = 1
	}
	for i := len(list.Items); i > minNum; i-- {
		if list.Items[i-1].Type == "folder" {
			path += list.Items[i-1].Name + "/"
		}
	}

	cache.GoCache.SetDefault(parentFileId+"path", path)

	return path, nil
}

func GetFile(url string, token string, rangeStr string) *http.Response {

	res, err := net.Get(url, token, rangeStr)
	if err == nil {
		return res
	}
	fmt.Println(err)
	return nil
	//net.GetProxy(w, req, url, token)
	//return []byte{}
}

func RefreshToken(refreshToken string) model.RefreshTokenModel {
	path := refreshToken
	if _, errs := os.Stat(path); errs == nil {
		buf, _ := ioutil.ReadFile(path)
		refreshToken = string(buf)
		if len(refreshToken) >= 32 {
			refreshToken = refreshToken[:32] // refreshToken is only 32 bit?? FIXME
		}
	}
	rs := net.Post(model.APIREFRESHTOKENURL, "", []byte(`{"refresh_token":"`+refreshToken+`"}`))
	var refresh model.RefreshTokenModel

	if len(rs) <= 0 {
		utils.Verbose(utils.VerboseLog, "刷新token失败")
		return refresh
	}

	err := json.Unmarshal(rs, &refresh)
	if err != nil {
		utils.Verbose(utils.VerboseLog, "刷新token失败,失败信息", err)
		utils.Verbose(utils.VerboseLog, "刷新token返回信息", refresh)
		return refresh
	}

	if refreshToken == refresh.RefreshToken {
		return refresh
	}

	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return refresh
	}
	if err != nil {
		utils.Verbose(utils.VerboseLog, "更新token文件失败,失败信息", err)
		return refresh
	}

	err = ioutil.WriteFile(path, []byte(refresh.RefreshToken), 0600)
	if err != nil {
		utils.Verbose(utils.VerboseLog, "更新token文件失败,失败信息", err)
	}

	return refresh
}

func RemoveTrash(token string, driveId string, fileId string, parentFileId string) bool {
	net.Post(model.APIREMOVETRASH, token, []byte(`{"drive_id":"`+driveId+`","file_id":"`+fileId+`"}`))
	//if len(rs) == 0 {
	//	cache.GoCache.Delete(parentFileId)
	//}
	cache.GoCache.Delete(parentFileId)
	return false
}

func ReName(token string, driveId string, newName string, fileId string) bool {
	rs := net.Post(model.APIFILEUPDATE, token, []byte(`{"drive_id":"`+driveId+`","file_id":"`+fileId+`","name":"`+newName+`","check_name_mode":"refuse"}`))
	var m model.ListModel
	e := json.Unmarshal(rs, &m)
	if e != nil {
		utils.Verbose(utils.VerboseLog, e)
	}
	cache.GoCache.Delete(m.ParentFileId)
	//utils.Verbose(utils.VerboseLog,string(rs))
	return true
}

// Walk 通过路径查找对应项目及所有子项目，当新建文件或文件夹时，也返回Not Found
func Walk(token string, driverId string, paths []string) (model.ListModel, model.FileListModel, error) {
	return WalkFolder(token, driverId, paths, false)
}

// WalkFolder 通过路径查找对应项目及所有子项目，当新建文件或文件夹时，也返回Not Found
func WalkFolder(token string, driverId string, paths []string, folderOnly bool) (model.ListModel, model.FileListModel, error) {
	var item = GetFileDetail(token, driverId, "root")
	var list model.FileListModel
	var err error
	err = errors.New("not found")
	if len(paths) == 0 || paths[0] == "" {
		list, _ = GetListA(token, driverId, "root", folderOnly)
		return item, list, nil
	}

	for j, path := range paths {
		item = GetFileDetail(token, driverId, item.FileId)
		if reflect.DeepEqual(item, model.ListModel{}) {
			return model.ListModel{}, model.FileListModel{}, err
		}

		list, _ = GetListA(token, driverId, item.FileId, folderOnly)
		for i, v := range list.Items {
			if _, ok := cache.GoCache.Get("FI_" + v.FileId); !ok {
				cache.GoCache.Set("FI_"+v.FileId, v, -1)
			}
			if j == 0 {
				if _, ok := cache.GoCache.Get("FID_" + v.Name); !ok {
					cache.GoCache.Set("FID_"+v.Name, v.FileId, -1)
				}
			} else {
				if _, ok := cache.GoCache.Get("FID_" + strings.Join(paths[:j], "/") + "/" + v.Name); !ok {
					cache.GoCache.Set("FID_"+strings.Join(paths[:j], "/")+"/"+v.Name, v.FileId, -1)
				}
			}
			//找到一个马上跳出去进入下一个路径
			if v.Name == path {
				item = v
				break
			}
			//如果是最后还没跳出，则资源不存在
			if i == len(list.Items)-1 {
				return model.ListModel{}, model.FileListModel{}, err
			}
		}
		if item.Name == path && j == len(paths)-1 {
			list, _ = GetListA(token, driverId, item.FileId, folderOnly)
			return item, list, nil
		}
	}
	return model.ListModel{}, model.FileListModel{}, err
}

func Search(token string, driveId string, name string, parentFileId string, Type string) model.FileListModel {
	var list model.FileListModel
	if c, ok := cache.GoCache.Get("SearchResult_" + parentFileId + name); ok {
		return c.(model.FileListModel)
	}
	if Type == "" {
		Type = "folder"
	}
	//{"drive_id":"67476554","query":"parent_file_id = \"61bdf6d66eced7c2c5324bb9a1fa54ae0d5e0f7d\" and (name = \"Screen Shot 2021-08-20 at 22.17.53.png\")","order_by":"name ASC","limit":100}
	body := net.Post(model.APISEARCH, token, []byte(`{"drive_id":"`+driveId+`","query":"parent_file_id = \"`+parentFileId+`\" and (name = \"`+name+`\") and (type=\"`+Type+`\")","order_by":"name ASC","limit":200}`))
	e := json.Unmarshal(body, &list)
	if e != nil {
		utils.Verbose(utils.VerboseLog, e)
	}
	if len(list.Items) > 0 {
		cache.GoCache.Set("SearchResult_"+parentFileId+name, list, -1)
	}
	return list
}
func MakeDir(token string, driveId string, name string, parentFileId string) model.CreateModel {
	rs := net.Post(model.APIMKDIR, token, []byte(`{"drive_id":"`+driveId+`","parent_file_id":"`+parentFileId+`","name":"`+name+`","check_name_mode":"refuse","type":"folder"}`))
	var fi model.CreateModel
	err := json.Unmarshal(rs, &fi)
	if err == nil {
		if fi.Name == name {
			cache.GoCache.Delete(parentFileId)
		}
		return fi
	}
	return fi
}

func GetFileDetail(token string, driveId string, fileId string) model.ListModel {
	if fileId != "root" {
		if va, ok := cache.GoCache.Get("FI_" + fileId); ok {
			if va != nil {
				return va.(model.ListModel)
			}

		}
	}

	rs := net.Post(model.APIFILEDETAIL, token, []byte(`{"drive_id":"`+driveId+`","file_id":"`+fileId+`"}`))
	var m model.ListModel
	e := json.Unmarshal(rs, &m)
	if e != nil {
		utils.Verbose(utils.VerboseLog, "❌   GetFileDetail Failed", e, string(rs))
	} else {
		cache.GoCache.Set("FI_"+fileId, e, -1)
	}
	return m
}

func BatchFile(token string, driveId string, fileId string, parentFileId string) bool {

	//	{
	//		"requests": ,
	//	"resource": "file"
	//	}

	var bodyJson string = `{"drive_id": "` + driveId + `","file_id": "` + fileId + `","to_drive_id": "` + driveId + `","to_parent_file_id": "` + parentFileId + `"}`
	var contentType string = `{"Content-Type": "application/json"}`

	var requests string = `{"requests":[{"body": ` + bodyJson + `,"headers": ` + contentType + `,"id": "` + fileId + `","method": "POST","url": "/file/move"}],"resource": "file"}`

	rs := net.Post(model.APIFILEBATCH, token, []byte(requests))
	if gjson.GetBytes(rs, "responses.0.friends").Num == 200 {
		cache.GoCache.Delete(parentFileId)
		cache.GoCache.Delete(fileId)
		return true
	}

	return false
}
func UpdateFileFolder(token string, driveId string, fileName string, parentFileId string) bool {

	//	{
	//		"requests": ,
	//	"resource": "file"
	//	}
	createData := `{"drive_id": "` + driveId + `","parent_file_id": "` + parentFileId + `","name": "` + fileName + `","check_name_mode": "refuse","type": "folder"}`
	net.Post(model.APIFILEUPLOAD, token, []byte(createData))
	// rs := net.Post(model.APIFILEUPLOAD, token, []byte(createData))
	// utils.Verbose(utils.VerboseLog,string(rs))
	//正确返回占星显示
	//	{"parent_file_id":"60794ad941ee2d8d24f843b7a0ffd80279927dfc","type":"folder","file_id":"613caeb4d5b1ba9fb4604d4aa5aef2b408ab3121","domain_id":"bj29","drive_id":"1662258","file_name":"1SDSDSD.png","encrypt_mode":"none"}
	//
	//	{
	//		"parent_file_id": "root",
	//		"part_info_list": [
	//	{
	//		"part_number": 1,
	//		"upload_url": "https://bj29.cn-beijing.data.alicloudccp.com/igQPcuUn%2F1662258%2F613a1091919bb599f4ac4917bfe16af6b7066795%2F613a10919ab3804e88e846ee9ea459de51d8f58d?partNumber=1&uploadId=BD8449BB161A4F54A1252E3B5B121641&x-oss-access-key-id=LTAIsE5mAn2F493Q&x-oss-expires=1631198881&x-oss-signature=wp2WCgyfqxZhJH%2BsPaw6XASRKXHa92p3e9NOjcN4Ui8%3D&x-oss-signature-version=OSS2",
	//		"internal_upload_url": "http://ccp-bj29-bj-1592982087.oss-cn-beijing-internal.aliyuncs.com/igQPcuUn%2F1662258%2F613a1091919bb599f4ac4917bfe16af6b7066795%2F613a10919ab3804e88e846ee9ea459de51d8f58d?partNumber=1&uploadId=BD8449BB161A4F54A1252E3B5B121641&x-oss-access-key-id=LTAIsE5mAn2F493Q&x-oss-expires=1631198881&x-oss-signature=wp2WCgyfqxZhJH%2BsPaw6XASRKXHa92p3e9NOjcN4Ui8%3D&x-oss-signature-version=OSS2",
	//		"content_type": ""
	//	}
	//],
	//	"upload_id": "BD8449BB161A4F54A1252E3B5B121641",
	//	"rapid_upload": false,
	//	"type": "file",
	//	"file_id": "613a1091919bb599f4ac4917bfe16af6b7066795",
	//	"domain_id": "bj29",
	//	"drive_id": "1662258",
	//	"file_name": "photo_1614943806132229.jpg",
	//	"encrypt_mode": "none",
	//	"location": "cn-beijing"
	//	}

	return false
}

func UpdateFileFile(token string, driveId string, fileName string, parentFileId string, size string, length int, contentHash string, proof string, flashUpload bool) ([]gjson.Result, string, string, bool) {

	if len(parentFileId) == 0 {
		parentFileId = "root"
	}

	var partStr string = "["
	for i := 0; i < length; i++ {
		partStr += `{"part_number":` + strconv.Itoa(i+1) + `},`
	}
	partStr = partStr[:len(partStr)-1]
	partStr += "]"

	var createData string = ""
	if flashUpload {
		createData = `{"drive_id":"` + driveId + `","part_info_list":` + partStr + `,"parent_file_id":"` + parentFileId + `","name":"` + fileName + `","type":"file","check_name_mode":"overwrite","size":` + size + `,"content_hash_name":"sha1","content_hash":"` + contentHash + `","proof_version":"v1","proof_code":"` + proof + `"}`

	} else {
		createData = `{"drive_id":"` + driveId + `","part_info_list":` + partStr + `,"parent_file_id":"` + parentFileId + `","name":"` + fileName + `","type":"file","check_name_mode":"overwrite","size":` + size + `,"content_hash_name":"","proof_version":"v1"}`
	}
	rs := net.Post(model.APIFILEUPLOAD, token, []byte(createData))
	rapidUpload := gjson.GetBytes(rs, "rapid_upload").Bool()
	if rapidUpload == true {
		return nil, gjson.GetBytes(rs, "upload_id").Str, gjson.GetBytes(rs, "file_id").Str, true
	}
	urlArr := gjson.GetBytes(rs, "part_info_list.#.upload_url").Array()
	if len(urlArr) == 0 {
		utils.Verbose(utils.VerboseLog, "❌  创建文件出错", string(rs))
	}
	return urlArr, gjson.GetBytes(rs, "upload_id").Str, gjson.GetBytes(rs, "file_id").Str, false

}
func UploadFile(url string, token string, data []byte) bool {
	//最多试15次
	for i := 0; i < 15; i++ {
		rs, status := net.Put(url, token, data)
		if len(rs) == 0 && status == 0 {
			return true
		} else {
			utils.Verbose(utils.VerboseLog, "❌  Upload Error: ", string(rs), " Retrying in 5 seconds")
			time.Sleep(5 * time.Second)
		}
	}
	return false
}
func UploadFileComplete(token string, driveId string, uploadId string, fileId string, parentId string) bool {

	createData := `{"drive_id": "` + driveId + `","file_id": "` + fileId + `","upload_id": "` + uploadId + `"}`

	rs := net.Post(model.APIFILECOMPLETE, token, []byte(createData))
	utils.Verbose(utils.VerboseLog, "⬆️  Upload Result:", gjson.GetBytes(rs, "file_id").Str, gjson.GetBytes(rs, "name").Str, gjson.GetBytes(rs, "size").Str)
	cache.GoCache.Delete(parentId)

	return false
}
func GetDownloadUrl(token string, driveId string, fileId string) string {

	postData := make(map[string]interface{})
	postData["drive_id"] = driveId
	postData["file_id"] = fileId

	data, _ := json.Marshal(postData)

	body := net.Post(model.APIFILEDOWNLOAD, token, data)
	return gjson.GetBytes(body, "url").Str

}
func GetBoxSize(token string) (string, string) {

	postData := make(map[string]interface{})

	data, _ := json.Marshal(postData)

	body := net.Post(model.APITOTLESIZE, token, data)
	return gjson.GetBytes(body, "personal_space_info.total_size").String(), gjson.GetBytes(body, "personal_space_info.used_size").String()

}
func GetUploadUrls(token string, driveId string, fileId string, uploadId string, length int) []gjson.Result {
	var partStr string = "["
	for i := 0; i < length; i++ {
		partStr += `{"part_number":` + strconv.Itoa(i+1) + `},`
	}
	partStr = partStr[:len(partStr)-1]
	partStr += "]"
	uploadRequest := `{"drive_id":"` + driveId + `","part_info_list":` + partStr + `,"file_id":"` + fileId + `","upload_id":"` + uploadId + `"}`
	rs := net.Post(model.APIFILEUPLOADURL, token, []byte(uploadRequest))
	//utils.Verbose(utils.VerboseLog,"ℹ️  GetUploadUrls", string(rs))
	return gjson.GetBytes(rs, "part_info_list.#.upload_url").Array()
}
