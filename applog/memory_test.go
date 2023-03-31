// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"fmt"
	"strings"

	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

var (
	bigMessage  = strings.Repeat("x", 100*1024)    // 100KiB
	hugeMessage = strings.Repeat("x", 2*1024*1024) // 2MiB
)

func (s *S) Test_MemoryLogService_AddWrapsOnLimit(c *check.C) {
	svc := memoryLogService{}
	for i := 0; i < 20; i++ {
		err := svc.Add("myapp", bigMessage, "tsuru", fmt.Sprintf("unit-%d", i))
		c.Assert(err, check.IsNil)
	}
	buffer := svc.getAppBuffer("myapp")
	c.Assert(buffer.length, check.Equals, 10)
	c.Assert(buffer.size <= defaultMaxAppBufferSize, check.Equals, true)
	c.Assert(buffer.start.prev, check.Equals, buffer.end)
	c.Assert(buffer.end.next, check.Equals, buffer.start)
	msgs, err := svc.List(context.TODO(), appTypes.ListLogArgs{Name: "myapp"})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, msgs, []appTypes.Applog{
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-10"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-11"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-12"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-13"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-14"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-15"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-16"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-17"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-18"},
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "unit-19"},
	})
}

func (s *S) Test_MemoryLogService_MessageLargerThanLimit(c *check.C) {
	svc := memoryLogService{}
	err := svc.Add("myapp", bigMessage, "tsuru", "avranakern")
	c.Assert(err, check.IsNil)
	err = svc.Add("myapp", hugeMessage, "tsuru", "portia")
	c.Assert(err, check.IsNil)
	buffer := svc.getAppBuffer("myapp")
	c.Assert(buffer.length, check.Equals, 1)
	c.Assert(buffer.size <= defaultMaxAppBufferSize, check.Equals, true)
	msgs, err := svc.List(context.TODO(), appTypes.ListLogArgs{Name: "myapp"})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, msgs, []appTypes.Applog{
		{Message: bigMessage, Name: "myapp", Source: "tsuru", Unit: "avranakern"},
	})
}

func (s *S) Test_MemoryLogService_MessagExactLimit(c *check.C) {
	svc := memoryLogService{}
	buffer := svc.getAppBuffer("myapp")
	err := svc.Add("myapp", bigMessage, "tsuru", "avranakern0")
	c.Assert(err, check.IsNil)
	newSize := defaultMaxAppBufferSize - (int(buffer.size) - len(bigMessage))
	newMessage := strings.Repeat("x", newSize)
	err = svc.Add("myapp", newMessage, "tsuru", "avranakern1")
	c.Assert(err, check.IsNil)
	err = svc.Add("myapp", newMessage, "tsuru", "avranakern2")
	c.Assert(err, check.IsNil)
	c.Assert(buffer.length, check.Equals, 1)
	c.Assert(buffer.size == defaultMaxAppBufferSize, check.Equals, true)
	msgs, err := svc.List(context.TODO(), appTypes.ListLogArgs{Name: "myapp"})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, msgs, []appTypes.Applog{
		{Message: newMessage, Name: "myapp", Source: "tsuru", Unit: "avranakern2"},
	})
}
