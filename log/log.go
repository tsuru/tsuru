package log

import "log"

var Target *log.Logger

func Fatal(v ...interface{}) {
	if Target != nil {
		Target.Fatal(v...)
	}
}

func Print(v ...interface{}) {
	if Target != nil {
		Target.Print(v...)
	}
}

func Printf(format string, v ...interface{}) {
	if Target != nil {
		Target.Printf(format, v...)
	}
}

func Panic(v ...interface{}) {
	if Target != nil {
		Target.Panic(v...)
	}
}

func Panicf(format string, v ...interface{}) {
	if Target != nil {
		Target.Panicf(format, v...)
	}
}
