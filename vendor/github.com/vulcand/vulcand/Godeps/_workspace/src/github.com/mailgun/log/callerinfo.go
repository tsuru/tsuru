package log

import (
	"os"
	"path/filepath"
	"runtime"
)

var (
	pid      = os.Getpid()
	hostname = "" // will be set in the init function
	appname  = filepath.Base(os.Args[0])
)

func init() {
	// determine the hostname but do not stop if it fails
	var err error
	if hostname, err = os.Hostname(); err != nil {
		hostname = "unknown_host"
	}
}

// CallerInfo encapsulates information about a piece of code that called a certain log function,
// such as file name, line number, etc.
type CallerInfo struct {
	FileName string
	FilePath string
	FuncName string
	LineNo   int
}

// getCallerInfo returns information about a certain log function invoker
// such as file name, function name and line number
func getCallerInfo(depth int) *CallerInfo {
	if pc, filePath, lineNo, ok := runtime.Caller(depth + 1); !ok {
		return &CallerInfo{"unknown_file", "unknown_path", "unknown_func", 0}
	} else {
		return &CallerInfo{filepath.Base(filePath), filePath, runtime.FuncForPC(pc).Name(), lineNo}
	}
}
