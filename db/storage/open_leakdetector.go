// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build leakdetector

package storage

import (
	"fmt"
	"runtime"
	"sync"

	"gopkg.in/mgo.v2"
)

var (
	pointerMap = map[string][2048]byte{}
	pointerMut sync.Mutex
)

func Open(addr, dbname string) (storage *Storage, err error) {
	sessionLock.RLock()
	if sessions[addr] == nil {
		sessionLock.RUnlock()
		sessionLock.Lock()
		if sessions[addr] == nil {
			sessions[addr], err = open(addr)
		}
		sessionLock.Unlock()
		if err != nil {
			return
		}
	} else {
		sessionLock.RUnlock()
	}
	cloned := sessions[addr].Clone()
	pointerAddr := fmt.Sprintf("%p", cloned)
	pointerMut.Lock()
	buf := pointerMap[pointerAddr]
	runtime.Stack(buf[:], false)
	pointerMap[pointerAddr] = buf
	pointerMut.Unlock()
	runtime.SetFinalizer(cloned, sessionFinalizer)
	storage = &Storage{
		session: cloned,
		dbname:  dbname,
	}
	return
}

func sessionFinalizer(session *mgo.Session) {
	ptr := fmt.Sprintf("%p", session)
	defer func() {
		recover()
		delete(pointerMap, ptr)
		pointerMut.Unlock()
	}()
	pointerMut.Lock()
	session.DB("tsuru").C("mycoll").Find(nil).Count()
	buf := pointerMap[ptr]
	fmt.Printf("\n********** LEAK **********\n%s\n********** ENDLEAK **********\n", string(buf[:]))
	session.Close()
}
