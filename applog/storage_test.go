package applog

import (
	"strconv"
	"sync"
	"time"

	appTypes "github.com/tsuru/tsuru/types/app"
	"gopkg.in/check.v1"
)

func (s *S) Test_LogService_Add(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	err = svc.Add("myapp", "last log msg", "tsuru", "outermachine")
	c.Assert(err, check.IsNil)
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[0].Source, check.Equals, "tsuru")
	c.Assert(logs[0].AppName, check.Equals, "myapp")
	c.Assert(logs[0].Unit, check.Equals, "outermachine")
}

func (s *S) Test_LogService_AddShouldAddOneRecordByLine(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	err = svc.Add("myapp", "last log msg\nfirst log", "tsuru", "outermachine")
	c.Assert(err, check.IsNil)
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "myapp"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 2)
	c.Assert(logs[0].Message, check.Equals, "last log msg")
	c.Assert(logs[1].Message, check.Equals, "first log")
}

func (s *S) Test_LogService_AddShouldNotLogBlankLines(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	err = svc.Add("ich", "some message", "tsuru", "machine")
	c.Assert(err, check.IsNil)
	err = svc.Add("ich", "", "", "")
	c.Assert(err, check.IsNil)
	logs, err := svc.List(appTypes.ListLogArgs{AppName: "ich"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
}

func (s *S) Test_LogService_AddWithListeners(c *check.C) {
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := newLogListener(svc, "myapp", appTypes.Applog{})
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.c {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	err = svc.Add("myapp", "last log msg", "tsuru", "machine")
	c.Assert(err, check.IsNil)
	done := make(chan bool, 1)
	q := make(chan bool)
	go func(quit chan bool) {
		for range time.Tick(1e3) {
			select {
			case <-quit:
				return
			default:
			}
			logs.Lock()
			if len(logs.l) == 1 {
				logs.Unlock()
				done <- true
				return
			}
			logs.Unlock()
		}
	}(q)
	select {
	case <-done:
	case <-time.After(2e9):
		defer close(q)
		c.Fatal("Timed out.")
	}
	logs.Lock()
	c.Assert(logs.l, check.HasLen, 1)
	log := logs.l[0]
	logs.Unlock()
	c.Assert(log.Message, check.Equals, "last log msg")
	c.Assert(log.Source, check.Equals, "tsuru")
	c.Assert(log.Unit, check.Equals, "machine")
}

func (s *S) Test_LogService_List(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		svc.Add("myapp", strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	svc.Add("myapp", "myapp log from circus", "circus", "rdaneel")
	logs, err := svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *S) Test_LogService_ListUnitFilter(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		svc.Add("app3", strconv.Itoa(i), "tsuru", "rdaneel")
		time.Sleep(1e6) // let the time flow
	}
	svc.Add("app3", "app3 log from circus", "circus", "rdaneel")
	svc.Add("app3", "app3 log from tsuru", "tsuru", "seldon")
	logs, err := svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "app3", Source: "tsuru", Unit: "rdaneel"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	for i := 5; i < 15; i++ {
		c.Check(logs[i-5].Message, check.Equals, strconv.Itoa(i))
		c.Check(logs[i-5].Source, check.Equals, "tsuru")
	}
}

func (s *S) Test_LogService_ListEmpty(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	logs, err := svc.List(appTypes.ListLogArgs{Limit: 10, AppName: "myapp", Source: "tsuru"})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.DeepEquals, []appTypes.Applog{})
}
