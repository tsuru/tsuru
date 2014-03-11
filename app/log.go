// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo/bson"
	"sync"
	"sync/atomic"
)

const (
	closed int32 = iota
	open
)

var listeners = struct {
	m map[string][]*LogListener
	sync.RWMutex
}{
	m: make(map[string][]*LogListener),
}

type LogListener struct {
	C       <-chan Applog
	c       chan Applog
	quit    chan byte
	state   int32
	appname string
}

func NewLogListener(a *App) *LogListener {
	c := make(chan Applog, 10)
	l := LogListener{C: c, c: c, state: open, appname: a.Name}
	l.quit = make(chan byte)
	listeners.Lock()
	list := listeners.m[l.appname]
	list = append(list, &l)
	listeners.m[a.Name] = list
	listeners.Unlock()
	return &l
}

func (l *LogListener) Close() error {
	if !atomic.CompareAndSwapInt32(&l.state, open, closed) {
		return errors.New("Already closed.")
	}
	listeners.Lock()
	defer listeners.Unlock()
	close(l.quit)
	close(l.c)
	list := listeners.m[l.appname]
	index := -1
	for i, listener := range list {
		if listener == l {
			index = i
			break
		}
	}
	if index > -1 {
		list[index], list[len(list)-1] = list[len(list)-1], list[index]
		listeners.m[l.appname] = list[:len(list)-1]
	}
	return nil
}

func notify(appName string, messages []interface{}) {
	var wg sync.WaitGroup
	listeners.RLock()
	ls := make([]*LogListener, len(listeners.m[appName]))
	copy(ls, listeners.m[appName])
	listeners.RUnlock()
	for _, l := range ls {
		wg.Add(1)
		go func(l *LogListener) {
			defer wg.Done()
			for _, msg := range messages {
				select {
				case <-l.quit:
					return
				default:
					defer func() {
						recover()
					}()
					l.c <- msg.(Applog)
				}
			}
		}(l)
	}
	wg.Wait()
}

// LogRemove removes the app log.
func LogRemove(a *App) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	//defer conn.Close()
	if a != nil {
		_, err = conn.Logs().RemoveAll(bson.M{"appname": a.Name})
	} else {
		_, err = conn.Logs().RemoveAll(nil)
	}
	return err
}
