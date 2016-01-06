package log

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 55647

	DefaultCategory = "go_logging"
)

type udpLogRecord struct {
	AppName   string  `json:"appname"`
	HostName  string  `json:"hostname"`
	LogLevel  string  `json:"logLevel"`
	FileName  string  `json:"filename"`
	FuncName  string  `json:"funcName"`
	LineNo    int     `json:"lineno"`
	Message   string  `json:"message"`
	Timestamp float64 `json:"timestamp"`
}

// udpLogger is a type of writerLogger that sends messages in a special format to a udplog server.
type udpLogger struct {
	*writerLogger // provides Writer() through embedding
}

func NewUDPLogger(conf Config) (Logger, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%v", DefaultHost, DefaultPort))
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}

	sev, err := SeverityFromString(conf.Severity)
	if err != nil {
		return nil, err
	}

	return &udpLogger{&writerLogger{sev, conn}}, nil
}

func (l *udpLogger) SetSeverity(sev Severity) {
	l.sev = sev
}

func (l *udpLogger) GetSeverity() Severity {
	return l.sev
}

func (l *udpLogger) FormatMessage(sev Severity, caller *CallerInfo, format string, args ...interface{}) string {
	rec := &udpLogRecord{
		AppName:   appname,
		HostName:  hostname,
		LogLevel:  sev.String(),
		FileName:  caller.FilePath,
		FuncName:  caller.FuncName,
		LineNo:    caller.LineNo,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: float64(time.Now().UnixNano()) / 1000000000,
	}

	dump, err := json.Marshal(rec)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s:%s", DefaultCategory, dump)
}
