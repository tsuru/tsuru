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

// SafeBuffer is a thread safe buffer.
type SafeBuffer struct {
	buf bytes.Buffer
	sync.Mutex
}

func (sb *SafeBuffer) Read(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) Write(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Write(p)
}

func (s *S) TestChannelFromWriter(c *C) {
	var buf SafeBuffer
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
	var buf SafeBuffer
	ch, errCh := ChannelFromWriter(&buf)
	close(ch)
	_, ok := <-errCh
	c.Assert(ok, Equals, false)
}

func (s *S) TestChannelFromReader(c *C) {
	var buf SafeBuffer
	messages := []Message{
		{Action: "delete", Args: []string{"everything"}},
		{Action: "rename", Args: []string{"old", "new"}},
		{Action: "destroy", Args: []string{"anything", "something", "otherthing"}},
	}
	encoder := gob.NewEncoder(&buf)
	for _, message := range messages {
		err := encoder.Encode(message)
		c.Assert(err, IsNil)
	}
	gotMessages := make([]Message, len(messages))
	ch, errCh := ChannelFromReader(&buf)
	for i := 0; i < len(messages); i++ {
		gotMessages[i] = <-ch
	}
	c.Assert(gotMessages, DeepEquals, messages)
	err := <-errCh
	c.Assert(err, IsNil)
	_, ok := <-ch
	c.Assert(ok, Equals, false)
	_, ok = <-errCh
	c.Assert(ok, Equals, false)
}
