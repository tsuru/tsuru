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

func Panic(v ...interface{}) {
	if Target != nil {
		Target.Panic(v...)
	}
}
