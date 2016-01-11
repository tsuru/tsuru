package log

import (
	"fmt"
	"io"
	"log/syslog"
)

// sysLogger logs messages to rsyslog MAIL_LOG facility.
type sysLogger struct {
	sev Severity

	debugW io.Writer
	infoW  io.Writer
	warnW  io.Writer
	errorW io.Writer
}

func NewSysLogger(conf Config) (Logger, error) {
	debugW, err := syslog.New(syslog.LOG_MAIL|syslog.LOG_DEBUG, appname)
	if err != nil {
		return nil, err
	}

	infoW, err := syslog.New(syslog.LOG_MAIL|syslog.LOG_INFO, appname)
	if err != nil {
		return nil, err
	}

	warnW, err := syslog.New(syslog.LOG_MAIL|syslog.LOG_WARNING, appname)
	if err != nil {
		return nil, err
	}

	errorW, err := syslog.New(syslog.LOG_MAIL|syslog.LOG_ERR, appname)
	if err != nil {
		return nil, err
	}

	sev, err := SeverityFromString(conf.Severity)
	if err != nil {
		return nil, err
	}

	return &sysLogger{sev, debugW, infoW, warnW, errorW}, nil
}

func (l *sysLogger) SetSeverity(sev Severity) {
	l.sev = sev
}

func (l *sysLogger) GetSeverity() Severity {
	return l.sev
}

func (l *sysLogger) Writer(sev Severity) io.Writer {
	// is this logger configured to log at the provided severity?
	if sev >= l.sev {
		// return an appropriate writer
		switch sev {
		case SeverityDebug:
			return l.debugW
		case SeverityInfo:
			return l.infoW
		case SeverityWarning:
			return l.warnW
		default:
			return l.errorW
		}
	}
	return nil
}

func (l *sysLogger) FormatMessage(sev Severity, caller *CallerInfo, format string, args ...interface{}) string {
	return fmt.Sprintf("%s [%s:%d] %s", sev, caller.FileName, caller.LineNo, fmt.Sprintf(format, args...))
}
