package log

import (
	"fmt"
	"os"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var Logger = logrus.New()

var stdFormatter *prefixed.TextFormatter  // 命令行输出格式
var fileFormatter *prefixed.TextFormatter // 文件输出格式

func init() {
	stdFormatter = &prefixed.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02.15:04:05.000000",
		ForceFormatting: true,
		ForceColors:     true,
		DisableColors:   false,
	}
	fileFormatter = &prefixed.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02.15:04:05.000000",
		ForceFormatting: true,
		ForceColors:     false,
		DisableColors:   false,
	}

	logPath := "./"
	logName := fmt.Sprintf("%s/log", logPath)
	writerFile, _ := rotatelogs.New(logName + "_%Y%m%d.log")
	writerStd := os.Stdout
	lfHook := lfshook.NewHook(lfshook.WriterMap{
		// logrus.DebugLevel: writer2,
		logrus.InfoLevel:  writerStd,
		logrus.WarnLevel:  writerStd,
		logrus.ErrorLevel: writerStd,
		logrus.FatalLevel: writerStd,
		logrus.PanicLevel: writerStd,
	}, stdFormatter)
	Logger.SetOutput(writerFile)
	Logger.AddHook(lfHook)
	Logger.SetFormatter(fileFormatter)
	Logger.SetLevel(logrus.DebugLevel)
}
