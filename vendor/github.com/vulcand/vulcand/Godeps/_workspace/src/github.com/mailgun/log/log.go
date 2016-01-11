package log

import (
	"fmt"
	"io"
)

type grouplogger struct {
	loggers []Logger
}

var gl grouplogger

// Supported log types.
const (
	Console = "console"
	Syslog  = "syslog"
	UDPLog  = "udplog"
)

// Logger is an interface that should be implemented by all loggers wishing to participate
// in the logger chain initialized by this package.
type Logger interface {
	// Writer returns logger's underlying io.Writer used to write log messages to.
	//
	// It may be, for example, the standard output for a console logger or a socket
	// connection for a UDP logger.
	//
	// Should return `nil` if the logger is not supposed to log at the specified severity.
	Writer(Severity) io.Writer

	// FormatMessage constructs and returns a final message that will go to the logger's
	// output channel.
	FormatMessage(Severity, *CallerInfo, string, ...interface{}) string

	// Sets a loggers current Severity level.
	SetSeverity(Severity)

	// Gets the current Severity level.
	GetSeverity() Severity
}

// Config represents a configuration of an individual logger.
type Config struct {
	// Name is a logger's identificator used to instantiate a proper logger type
	// from a config.
	Name string

	// Severity indicates the minimum severity a logger will be logging messages at.
	Severity string
}

// Init initializes the logging package with the provided loggers.
func Init(l ...Logger) {
	for _, logger := range l {
		gl.loggers = append(gl.loggers, logger)
	}
}

// InitWithConfig instantiates loggers based on the provided configs and initializes
// the package with them.
func InitWithConfig(configs ...Config) error {
	for _, config := range configs {
		l, err := NewLogger(config)
		if err != nil {
			return err
		}
		gl.loggers = append(gl.loggers, l)
	}
	return nil
}

// NewLogger makes a proper logger from the given configuration.
func NewLogger(config Config) (Logger, error) {
	switch config.Name {
	case Console:
		return NewConsoleLogger(config)
	case Syslog:
		return NewSysLogger(config)
	case UDPLog:
		return NewUDPLogger(config)
	}
	return nil, fmt.Errorf("unknown logger: %v", config)
}

func SetSeverity(sev Severity) {
	gl.SetSeverity(sev)
}

func (gl *grouplogger) SetSeverity(sev Severity) {
	for _, logger := range gl.loggers {
		logger.SetSeverity(sev)
	}
}

// Debugf logs to the DEBUG log.
func Debugf(format string, args ...interface{}) {
	gl.Debugf(format, args...)
}

func (gl *grouplogger) Debugf(format string, args ...interface{}) {
	for _, logger := range gl.loggers {
		writeMessage(logger, 1, SeverityDebug, format, args...)
	}
}

// Infof logs to the INFO log.
func Infof(format string, args ...interface{}) {
	gl.Infof(format, args...)
}

func (gl *grouplogger) Infof(format string, args ...interface{}) {
	for _, logger := range gl.loggers {
		writeMessage(logger, 1, SeverityInfo, format, args...)
	}
}

// Warningf logs to the WARN and INFO logs.
func Warningf(format string, args ...interface{}) {
	gl.Warningf(format, args...)
}

func (gl *grouplogger) Warningf(format string, args ...interface{}) {
	for _, logger := range gl.loggers {
		writeMessage(logger, 1, SeverityWarning, format, args...)
	}
}

// Errorf logs to the ERROR, WARN, and INFO logs.
func Errorf(format string, args ...interface{}) {
	gl.Errorf(format, args...)
}

func (gl *grouplogger) Errorf(format string, args ...interface{}) {
	for _, logger := range gl.loggers {
		writeMessage(logger, 1, SeverityError, format, args...)
	}
}

func writeMessage(logger Logger, callDepth int, sev Severity, format string, args ...interface{}) {
	caller := getCallerInfo(callDepth + 1)
	if w := logger.Writer(sev); w != nil {
		message := logger.FormatMessage(sev, caller, format, args...)
		io.WriteString(w, message)
	}
}

type LogWriter interface {
	Infof(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

func GetGlobalLogger() LogWriter {
	return &gl
}
