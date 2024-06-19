package debug

import (
	"io"
	"log"
	"os"

	"github.com/natefinch/lumberjack"
)

const (
	envEnableLog = "GO_PROMPT_ENABLE_LOG"
	logFileName  = "go-prompt.log"
	maxSize      = 1 // log file size in MB
	maxBackup    = 3 // log file backup number
)

var (
	logWriter io.WriteCloser
	logger    *log.Logger
)

func init() {
	if e := os.Getenv(envEnableLog); e == "true" || e == "1" {
		logWriter = &lumberjack.Logger{
			Filename:   logFileName,
			MaxSize:    maxSize,
			MaxBackups: maxBackup,
			Compress:   true,
			LocalTime:  true,
		}
		logger = log.New(logWriter, "", log.LstdFlags|log.Llongfile)

	} else {
		logger = log.New(io.Discard, "", log.LstdFlags|log.Llongfile)
	}

}

// Teardown to close logfile
func Teardown() {
	if logWriter == nil {
		return
	}
	_ = logWriter.Close()
}

func writeWithSync(calldepth int, msg string) {
	calldepth++
	_ = logger.Output(calldepth, msg)
}

// Log to output message
func Log(msg string) {
	calldepth := 2
	writeWithSync(calldepth, msg)
}
