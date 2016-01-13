package log

import (
	"fmt"
	"io"
	"os"
	"time"
)

// writerLogger is a generic type of a logger that sends messages to the underlying io.Writer.
type writerLogger struct {
	sev Severity

	w io.Writer
}

func (l *writerLogger) Writer(sev Severity) io.Writer {
	// is this logger configured to log at the provided severity?
	if sev >= l.sev {
		return l.w
	}
	return nil
}

func (l *writerLogger) SetSeverity(sev Severity) {
	l.sev = sev
}

func (l *writerLogger) GetSeverity() Severity {
	return l.sev
}

// consoleLogger is a type of writerLogger that sends messages to the standard output.
type consoleLogger struct {
	*writerLogger // provides Writer() through embedding
}

func NewConsoleLogger(conf Config) (Logger, error) {
	sev, err := SeverityFromString(conf.Severity)
	if err != nil {
		return nil, err
	}
	return &consoleLogger{&writerLogger{sev, os.Stdout}}, nil
}

func (l *consoleLogger) FormatMessage(sev Severity, caller *CallerInfo, format string, args ...interface{}) string {
	return fmt.Sprintf("%v %s %s PID:%d [%s:%d:%s] %s\n",
		time.Now().UTC().Format(time.StampMilli), appname, sev, pid, caller.FileName, caller.LineNo, caller.FuncName, fmt.Sprintf(format, args...))
}
