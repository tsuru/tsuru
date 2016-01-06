// Copyright 2015 monsterqueue authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"log"
	"os"
	"sync"
)

var (
	logger *log.Logger
	lock   sync.RWMutex
	debug  bool
)

func init() {
	SetLogger(nil)
}

func SetLogger(l *log.Logger) {
	lock.Lock()
	defer lock.Unlock()
	if l == nil {
		l = log.New(os.Stderr, "", log.LstdFlags)
	}
	logger = l
}

func SetDebug(d bool) {
	debug = d
}

func Debugf(format string, args ...interface{}) {
	lock.RLock()
	defer lock.RUnlock()
	if debug {
		logger.Printf(fmt.Sprintf("[monsterqueue][debug] %s", format), args...)
	}
}

func Errorf(format string, args ...interface{}) {
	lock.RLock()
	defer lock.RUnlock()
	logger.Printf(fmt.Sprintf("[monsterqueue][error] %s", format), args...)
}
