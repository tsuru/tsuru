// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netqueue

import (
	"bytes"
	"encoding/gob"
	. "launchpad.net/gocheck"
	"sync"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestChannelFromWriter(c *C) {
	var buf bytes.Buffer
	message := Message{
		Action: "delete",
		Args:   []string{"everything"},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	ch, _ := ChannelFromWriter(&buf)
	go func() {
		ch <- message
		wg.Done()
	}()
	wg.Wait()
	var decodedMessage Message
	decoder := gob.NewDecoder(&buf)
	err := decoder.Decode(&decodedMessage)
	c.Assert(err, IsNil)
	c.Assert(decodedMessage, DeepEquals, message)
}

func (s *S) TestClosesErrChanIfClientCloseMessageChannel(c *C) {
	var buf bytes.Buffer
	ch, errCh := ChannelFromWriter(&buf)
	close(ch)
	_, ok := <-errCh
	c.Assert(ok, Equals, false)
}
