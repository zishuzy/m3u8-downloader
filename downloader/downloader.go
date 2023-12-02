package downloader

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io/ioutil"
	"m3u8-downloader/log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/levigross/grequests"
	"github.com/sirupsen/logrus"
)

type M3u8Info struct {
	Url      string // m3u8 的url
	Path     string // m3u8 下属 ts 保存的临时路径
	FilePath string // 合并 ts 最终文件路径

	Body    string      // m3u8 的body
	Key     string      // 解密Key
	SubM3u8 []*M3u8Info // 子 m3u8
	Tslist  []*TsInfo   // ts 文件信息指针
}

type TsInfo struct {
	Url  string // ts 的url
	Name string // ts 文件名称
}

const (
	kRequestTimeout = 10 * time.Second
)

var (
	// proxy, _ = url.Parse("http://127.0.0.1:10809")
	ro = &grequests.RequestOptions{
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.141 Safari/537.36",
		RequestTimeout: kRequestTimeout,
		Headers: map[string]string{
			"Connection":      "keep-alive",
			"Accept":          "*/*",
			"Accept-Encoding": "*",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		},
		// Proxies: map[string]*url.URL{
		// 	"http":  proxy,
		// 	"https": proxy,
		// },
	}
	g_urlType = 2
)

func DownloadOfM3u8(m3u8list []M3u8Info, url_type, max_thread int) {
	g_urlType = url_type
	maxMainThread := 3 // 批量下载时同时下载集数
	var wg sync.WaitGroup
	limiter := make(chan int, maxMainThread)

	for i := 0; i < len(m3u8list); i++ {
		wg.Add(1)
		limiter <- 1
		go func(m3u8 M3u8Info, max_thread int) {
			defer func() {
				wg.Done()
				<-limiter
			}()
			log.Logger.WithFields(logrus.Fields{
				"u3m8url": m3u8.Url,
				"path":    m3u8.Path,
			}).Info("start download m3u8...")
			completeM3u8Info(&m3u8)
			downloadOfM3u8(m3u8, max_thread)
		}(m3u8list[i], max_thread)

	}
	wg.Wait()
}

func completeM3u8Info(m3u8 *M3u8Info) {
	if m3u8.Url == "" {
		log.Logger.Error("m3u8 url is empty!!!")
		return
	}
	m3u8.Body = getUrlBody(m3u8.Url)
	m3u8.Key = getM3u8Key(m3u8.Url, &m3u8.Body)
	m3u8.Tslist = getM3u8Tslist(m3u8.Url, &m3u8.Body)
	m3u8UrlPaths := getM3u8Url(m3u8.Url, &m3u8.Body)
	for i := 0; i < len(m3u8UrlPaths); i++ {
		filenameWithSuffix := path.Base(m3u8.FilePath)
		fileSuffix := path.Ext(filenameWithSuffix)
		fileName := strings.TrimSuffix(filenameWithSuffix, fileSuffix)

		m3u8Sub := M3u8Info{
			Url:      m3u8UrlPaths[i],
			Path:     filepath.Join(m3u8.Path, strconv.Itoa(i)),
			FilePath: filepath.Join(m3u8.Path, fileName+"_"+strconv.Itoa(i)+fileSuffix),
		}
		completeM3u8Info(&m3u8Sub)
		m3u8.SubM3u8 = append(m3u8.SubM3u8, &m3u8Sub)
	}
}

func getM3u8Url(host string, body *string) []string {
	var urlPathList []string
	lines := strings.Split(*body, "\n")
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "#") {
			if !strings.Contains(line, ".ts") {
				strUrl := getRealUrl(host, line)
				urlPathList = append(urlPathList, strUrl)
			}
		}
	}
	return urlPathList
}

func getM3u8Key(host string, body *string) string {
	lines := strings.Split(*body, "\n")
	var keyBody string
	for _, line := range lines {
		if strings.Contains(line, "#EXT-X-KEY") {
			indexStart := strings.Index(line, "URI")
			indexEnd := strings.LastIndex(line, "\"")
			strKeyUrl := strings.Split(line[indexStart:indexEnd], "\"")[1]
			if !strings.Contains(line, "http") {
				strKeyUrl = getRealUrl(host, strKeyUrl)
				// if strKeyUrl[0] == '/' {
				// 	strKeyUrl = fmt.Sprintf("%s%s", host, strKeyUrl)
				// } else {
				// 	strKeyUrl = fmt.Sprintf("%s/%s", host, strKeyUrl)
				// }
			}
			keyBody = getUrlBody(strKeyUrl)
			// 这里假设一个m3u8中只有一个key
			break
		}
	}
	return keyBody
}

func getM3u8Tslist(host string, body *string) []*TsInfo {
	var pTsList []*TsInfo
	lines := strings.Split(*body, "\n")
	tsIndex := 0
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "#") && strings.Contains(line, ".ts") {
			var tsUrl string
			if strings.HasPrefix(line, "http") {
				tsUrl = line
			} else {
				tsUrl = getRealUrl(host, line)
			}
			// if line[0] == '/' {
			// 	tsUrl = fmt.Sprintf("%s%s", host, line)
			// } else {
			// 	tsUrl = fmt.Sprintf("%s/%s", host, line)
			// }
			log.Logger.Debug("tsUrl:", tsUrl)
			ts := TsInfo{
				Url:  tsUrl,
				Name: fmt.Sprintf("%05d.ts", tsIndex),
			}
			pTsList = append(pTsList, &ts)
			tsIndex++
		}
	}
	return pTsList
}

func downloadOfM3u8(m3u8 M3u8Info, max_thread int) {
	if isExist, _ := pathExists(m3u8.Path); !isExist {
		os.MkdirAll(m3u8.Path, os.ModePerm)
	}
	for i := 0; i < len(m3u8.SubM3u8); i++ {
		downloadOfM3u8(*m3u8.SubM3u8[i], max_thread)
	}
	downloadTsList(m3u8.Tslist, m3u8.Path, m3u8.Key, max_thread)

	if len(m3u8.Tslist) == 0 {
		moveMp4File(m3u8.Path)
	} else {
		mergeTsFile(m3u8.Path, m3u8.FilePath)
	}
	// os.RemoveAll(m3u8.Path)
}

func downloadTsList(tsList []*TsInfo, save_path, key string, max_thread int) {
	retryMax := 5 // 重试次数

	var wg sync.WaitGroup
	limiter := make(chan int, max_thread)
	downloadCount := 0

	for _, ts := range tsList {
		wg.Add(1)
		limiter <- 1
		go func(ts TsInfo, save_path, key string, retryies int) {
			defer func() {
				wg.Done()
				<-limiter
			}()
			log.Logger.WithFields(logrus.Fields{
				"tsurl": ts.Url,
				"name":  ts.Name,
				"path":  save_path,
			}).Debug("download ts...")
			downloadTsFile(ts, save_path, key, retryies)
			downloadCount++
			// progressBar(downloadCount, len(tsList))
			return
		}(*ts, save_path, key, retryMax)
	}
	wg.Wait()
}

func downloadTsFile(ts TsInfo, save_path, key string, retries int) {
	defer func() {
		if r := recover(); r != nil {
			downloadTsFile(ts, save_path, key, retries-1)
		}
	}()

	filePath := fmt.Sprintf("%s/%s", save_path, ts.Name)
	if isExist, _ := pathExists(filePath); isExist {
		log.Logger.Debugf("file[%s] is exist.", filePath)
		return
	}

	resp, err := http.Get(ts.Url)
	if err != nil {
		if retries > 0 {
			downloadTsFile(ts, save_path, key, retries-1)
			return
		} else {
			log.Logger.Error("urlfile[%s] download falied", ts.Url)
			return
		}
	}

	origData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if retries > 0 {
			downloadTsFile(ts, save_path, key, retries-1)
			return
		} else {
			log.Logger.Error("urlfile[%s] download falied", ts.Url)
			return
		}
	}
	defer resp.Body.Close()

	if len(origData) == 0 {
		log.Logger.WithFields(logrus.Fields{
			"tsUrl":  ts.Url,
			"tsName": ts.Name,
		}).Debug("ts is empty")
		downloadTsFile(ts, save_path, key, retries-1)
		return
	}

	if key != "" {
		// 解密ts文件 aes 128 cbc pack5
		origData, err = aesDecrypt(origData, []byte(key))
		if err != nil {
			downloadTsFile(ts, save_path, key, retries-1)
			return
		}
	}

	// https://en.wikipedia.org/wiki/MPEG_transport_stream
	// Some TS files do not start with SyncByte 0x47, they can not be played after merging,
	// Need to remove the bytes before the SyncByte 0x47(71).
	syncByte := uint8(71) //0x47
	bLen := len(origData)
	for j := 0; j < bLen; j++ {
		if origData[j] == syncByte {
			origData = origData[j:]
			break
		}
	}

	ioutil.WriteFile(filePath, origData, 0666)
}

func getUrlBody(u string) string {
	retryMax := 5 // 重试次数
	return getUrlBodyRetry(u, retryMax)
}

func getUrlBodyRetry(u string, retries int) (strBody string) {
	if retries < 0 {
		log.Logger.Panicf("get url[%s] body failed.", u)
	}
	// resp, err := http.Get(u)
	// if err == nil {
	// 	origData, err := ioutil.ReadAll(resp.Body)
	// 	if err == nil {
	// 		strBody = string(origData)
	// 	} else {
	// 		strBody = getUrlBodyRetry(u, retries-1)
	// 	}
	// } else {
	// 	strBody = getUrlBodyRetry(u, retries-1)
	// }

	r, err := grequests.Get(u, ro)
	if err == nil && r.Ok {
		strBody = r.String()
	} else {
		strBody = getUrlBodyRetry(u, retries-1)
	}
	return
}

func aesDecrypt(crypted, key []byte, ivs ...[]byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	var iv []byte
	if len(ivs) == 0 {
		iv = key
	} else {
		iv = ivs[0]
	}
	blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = pkcs7UnPadding(origData)
	return origData, nil
}

func pkcs7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func getRealUrl(header_url, tail_url string) string {

	strRealUrl := ""
	if g_urlType == 0 {
		strRealUrl = getRealUrlHost(header_url, tail_url)
	} else if g_urlType == 1 {
		strRealUrl = getRealUrlAdd(header_url, tail_url)
	} else {
		strRealUrl = getRealUrlAuto(header_url, tail_url)
	}
	return strRealUrl
}

func getRealUrlHost(header_url, tail_url string) string {

	strHeaderHost := getUrlHost(header_url)
	strRealUrl := strHeaderHost + tail_url

	return strRealUrl
}

func getRealUrlAdd(header_url, tail_url string) string {

	strHeaderHost := strings.Trim(getUrlHost(header_url), "/")
	strHeaderPath := strings.Trim(getUrlPath(header_url), "/")
	listHeaderPath := strings.Split(strHeaderPath, "/")
	listHeaderPath = listHeaderPath[0 : len(listHeaderPath)-1]
	strRealUrl := strHeaderHost + "/" + strings.Join(listHeaderPath, "/") + "/" + strings.Trim(tail_url, "/")
	return strRealUrl
}

func getRealUrlAuto(header_url, tail_url string) string {
	strHeaderHost := getUrlHost(header_url)
	strHeaderPath := getUrlPath(header_url)

	listHeaderPath := strings.Split(strHeaderPath, "/")
	listHeaderPath = listHeaderPath[0 : len(listHeaderPath)-1]
	listTailPath := strings.Split(tail_url, "/")

	strRealUrl := strHeaderHost
	nTailIndex := 0
	for nHeaderIndex := 0; nHeaderIndex < len(listHeaderPath) && nTailIndex < len(listTailPath); {
		if listHeaderPath[nHeaderIndex] == "" {
			nHeaderIndex++
			continue
		}
		if listTailPath[nTailIndex] == "" {
			nTailIndex++
			continue
		}
		strRealUrl += "/" + listHeaderPath[nHeaderIndex]
		if listHeaderPath[nHeaderIndex] == listTailPath[nTailIndex] {
			nTailIndex++
		}
		nHeaderIndex++
	}
	for i := nTailIndex; i < len(listTailPath); i++ {
		strRealUrl += "/" + listTailPath[i]
	}
	return strRealUrl
}

func getUrlHost(strurl string) string {
	urlInfo, err := url.Parse(strurl)
	checkErr(err)
	urlHost := urlInfo.Scheme + "://" + urlInfo.Host
	return urlHost
}

func getUrlPath(strurl string) string {
	urlInfo, err := url.Parse(strurl)
	checkErr(err)
	return urlInfo.Path
}

func getParentPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "/" {
		return "/"
	}
	index := strings.LastIndex(path, "/")
	return path[0:index]
}

func checkErr(err error) {
	if err != nil {
		log.Logger.Panic(err)
	}
}

func execShell(s string) {
	log.Logger.Debug("cmd:", s)
	cmd := exec.Command("/bin/bash", "-c", s)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	log.Logger.Debugf("cmd result[%s]", out.String())
	checkErr(err)
}

func moveMp4File(path string) {
	files, _ := ioutil.ReadDir(path)
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".mp4") {
			oldPath := filepath.Join(path, f.Name())
			newPath := filepath.Join(filepath.Dir(filepath.Dir(oldPath)), f.Name())
			if isExist, _ := pathExists(newPath); !isExist {
				log.Logger.Debugf("rename file[%s] -> file[%s]", oldPath, newPath)
				os.Rename(oldPath, newPath)
			}
		}
	}
}

func mergeTsFile(path, outPath string) {
	if isExist, _ := pathExists(outPath); !isExist {
		os.Chdir(path)
		log.Logger.Debugf("cd %s", path)
		cmd := `cat *.ts >> merge.tmp`
		execShell(cmd)
		os.Rename("merge.tmp", outPath)
		log.Logger.Debugf("rename file[merge.tmp] -> file[%s]", outPath)
	}
}

func progressBar(value, size int) {
	fi := float64(value) / float64(size) * 100
	i := int(fi)

	if value == size {
		log.Logger.Infof("%3d%% [%s]\n", i, getS(i, "#")+getS(100-i, " "))
		// fmt.Fprintf(log.Logger.Out, "%d%% [%s]\n", i, getS(i, "#")+getS(100-i, " "))
	} else {
		log.Logger.Infof("%3d%% [%s]\r", i, getS(i, "#")+getS(100-i, " "))
		// fmt.Fprintf(log.Logger.Out, "%d%% [%s]\r", i, getS(i, "#")+getS(100-i, " "))
	}
}

func getS(n int, char string) (s string) {
	for i := 1; i <= n; i++ {
		s += char
	}
	return
}
