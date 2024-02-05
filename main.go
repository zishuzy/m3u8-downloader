package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zishuzy/m3u8-downloader/downloader"
	"github.com/zishuzy/m3u8-downloader/log"
)

var (
	//命令行参数
	urlFlag = flag.String("u", "", "m3u8文件的url，和 -f 互斥")
	fFlag   = flag.String("f", "", "文件路径，批量下载m3u8，和 -u 互斥")
	tFlag   = flag.String("t", "url_3", "拼接url的方式: \n\turl_1: https://xxx/000/111/222.index + 111/222/001.ts = https://xxx/111/222/001.ts\n\turl_2: https://xxx/000/111/222/main.index + 111/222/001.ts = https://xxx/000/111/222/111/222/001.ts\n\turl_3: https://xxx/000/111/222/main.index + 111/222/001.ts = https://xxx/000/111/222/001.ts")
	nFlag   = flag.Int("n", 16, "下载线程数")
	oFlag   = flag.String("o", "output", "下载文件的路径")
	mFlag   = flag.String("m", "video", "最后合并成视频文件的文件名")

	g_m3u8Url     string
	g_m3u8urlFile string
	g_maxThreads  int
	g_outPath     string
	g_outName     string
	g_urlType     string
)

func init() {
	//解析命令行参数
	flag.Parse()
	g_m3u8Url = *urlFlag
	g_m3u8urlFile = *fFlag
	g_maxThreads = *nFlag
	g_outPath = *oFlag
	g_outName = *mFlag
	g_urlType = *tFlag
}

func main() {
	msgTpl := "[功能]: 针对m3u8文件的下载器\n[提醒]: 如果进度条中途下载失败，可重复执行"
	fmt.Println(msgTpl)

	if g_m3u8Url != "" {
		if !strings.HasPrefix(g_m3u8Url, "http") {
			flag.Usage()
			return
		}
		// 如果检测到 g_m3u8Url ，则清空 g_m3u8urlFile
		g_m3u8urlFile = ""
	} else if g_m3u8urlFile == "" {
		flag.Usage()
		return
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	nowTime := time.Now()

	if g_outPath[0] != '/' {
		pwd, _ := os.Getwd()
		downloadPath := fmt.Sprintf("%s/%s", pwd, g_outPath)
		if isExist, _ := pathExists(downloadPath); !isExist {
			os.MkdirAll(downloadPath, os.ModePerm)
			log.Logger.Infof("mkdir path[%s]", downloadPath)
		}
		g_outPath = downloadPath
	}

	var m3u8List []downloader.M3u8Info
	if g_m3u8Url != "" {
		m3u8Info := downloader.M3u8Info{
			Url:      g_m3u8Url,
			Path:     fmt.Sprintf("%s/%s", g_outPath, g_outName),
			FilePath: fmt.Sprintf("%s/%s.mp4", g_outPath, g_outName),
		}
		m3u8List = append(m3u8List, m3u8Info)
	}

	if g_m3u8urlFile != "" {
		if isExist, _ := pathExists(g_m3u8urlFile); !isExist {
			fmt.Printf("file[%s] is not exist", g_m3u8urlFile)
			return
		}
		m3u8List = parseM3U8file(g_m3u8urlFile)
	}
	url_type := 2
	if g_urlType == "url_1" {
		url_type = 0
	} else if g_urlType == "url_2" {
		url_type = 1
	} else {
		url_type = 2
	}
	downloader.DownloadOfM3u8(m3u8List, url_type, g_maxThreads)

	// removeSubDir(g_outPath)
	log.Logger.Info("完成，耗时:", time.Now().Sub(nowTime))
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

func parseM3U8file(filePath string) (m3u8List []downloader.M3u8Info) {
	file, err := os.Open(filePath)
	checkErr(err)
	defer file.Close()

	br := bufio.NewReader(file)
	index := 0
	for {
		b, _, c := br.ReadLine()
		if c == io.EOF {
			break
		}
		line := string(b)
		if line == "" {
			continue
		}

		m3u8 := downloader.M3u8Info{
			Url:      line,
			Path:     fmt.Sprintf("%s/%s_%d", g_outPath, g_outName, index),
			FilePath: fmt.Sprintf("%s/%s_%d.mp4", g_outPath, g_outName, index),
		}
		m3u8List = append(m3u8List, m3u8)
		index++
	}
	return m3u8List
}

// 删除 path 目录下的所有文件夹
func removeSubDir(path string) {
	files, _ := ioutil.ReadDir(path)
	for _, f := range files {
		if f.IsDir() {
			rmPath := filepath.Join(path, f.Name())
			os.RemoveAll(rmPath)
			log.Logger.Debugf("rmpath[%s]", rmPath)
		}
	}
}

func checkErr(err error) {
	if err != nil {
		log.Logger.Panic(err)
	}
}
