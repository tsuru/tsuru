// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"sync"
)

var _ ActionLimiter = &LocalLimiter{}

type ActionLimiter interface {
	SetLimit(uint)
	Add(action string)
	Done(action string)
}

type LocalLimiter struct {
	sync.Mutex
	chMap map[string]chan struct{}
	limit uint
}

func (l *LocalLimiter) SetLimit(i uint) {
	l.limit = i
	l.chMap = nil
	if i != 0 {
		l.chMap = make(map[string]chan struct{})
	}
}

func (l *LocalLimiter) Add(action string) {
	l.Lock()
	if l.chMap == nil {
		l.Unlock()
		return
	}
	var limitChan chan struct{}
	if l.chMap[action] == nil {
		l.chMap[action] = make(chan struct{}, l.limit)
	}
	limitChan = l.chMap[action]
	l.Unlock()
	limitChan <- struct{}{}
}

func (l *LocalLimiter) Done(action string) {
	l.Lock()
	var limitChan chan struct{}
	if l.chMap == nil || l.chMap[action] == nil {
		l.Unlock()
		return
	}
	limitChan = l.chMap[action]
	l.Unlock()
	<-limitChan
}

type NoopLimiter struct{}

func (NoopLimiter) SetLimit(i uint) {}

func (NoopLimiter) Add(action string) {}

func (NoopLimiter) Done(action string) {}
