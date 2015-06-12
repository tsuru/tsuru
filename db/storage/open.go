// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !leakdetector

package storage

import (
	"runtime"

	"gopkg.in/mgo.v2"
)

// Open dials to the MongoDB database, and return the connection (represented
// by the type Storage).
//
// addr is a MongoDB connection URI, and dbname is the name of the database.
//
// This function returns a pointer to a Storage, or a non-nil error in case of
// any failure.
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
	runtime.SetFinalizer(cloned, sessionFinalizer)
	storage = &Storage{
		session: cloned,
		dbname:  dbname,
	}
	return
}

func sessionFinalizer(session *mgo.Session) {
	session.Close()
}
