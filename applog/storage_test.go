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
	l, err := svc.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
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

func addLog(c *check.C, svc appTypes.AppLogService, appName, message, source, unit string) {
	err := svc.Add(appName, message, source, unit)
	c.Assert(err, check.IsNil)
}

func (s *S) TestWatch(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := svc.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	addLog(c, svc, "myapp", "123", "", "")
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "123")
	addLog(c, svc, "myapp", "456", "", "")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "456")
}

func (s *S) TestWatchFiltered(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := svc.Watch("myapp", "web", "u1")
	c.Assert(err, check.IsNil)
	defer l.Close()
	c.Assert(l.Chan(), check.NotNil)
	logs := []appTypes.Applog{
		{Message: "1", Source: "web", Unit: "u1"},
		{Message: "2", Source: "worker", Unit: "u1"},
		{Message: "3", Source: "web", Unit: "u1"},
		{Message: "4", Source: "web", Unit: "u2"},
		{Message: "5", Source: "web", Unit: "u1"},
	}
	for _, log := range logs {
		addLog(c, svc, "myapp", log.Message, log.Source, log.Unit)
	}
	logMsg := <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "1")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "3")
	logMsg = <-l.Chan()
	c.Assert(logMsg.Message, check.Equals, "5")
}

func (s *S) TestWatchClosingChannel(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := svc.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	c.Assert(l.Chan(), check.NotNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestWatchClose(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	l, err := svc.Watch("myapp", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	_, ok := <-l.Chan()
	c.Assert(ok, check.Equals, false)
}

func (s *S) TestWatchDoubleClose(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := svc.Watch("yourapp", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	l.Close()
}

func (s *S) TestWatchNotify(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	l, err := svc.Watch("fade", "", "")
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "some", AppName: "fade"},
		{Message: "This program has performed an illegal operation.", Source: "tsuru", Unit: "some", AppName: "fade"},
	}
	for _, log := range ms {
		addLog(c, svc, "fade", log.Message, log.Source, log.Unit)
	}
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
			if len(logs.l) == 2 {
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
	defer logs.Unlock()
	compareLogsNoDate(c, logs.l, ms)
}

func (s *S) TestWatchNotifyFiltered(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	var logs struct {
		l []appTypes.Applog
		sync.Mutex
	}
	l, err := svc.Watch("fade", "tsuru", "unit1")
	c.Assert(err, check.IsNil)
	defer l.Close()
	go func() {
		for log := range l.Chan() {
			logs.Lock()
			logs.l = append(logs.l, log)
			logs.Unlock()
		}
	}()
	ms := []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1"},
		{Message: "This program has performed an illegal operation.", Source: "other", Unit: "unit1"},
		{Message: "Last one.", Source: "tsuru", Unit: "unit2"},
	}
	for _, log := range ms {
		addLog(c, svc, "fade", log.Message, log.Source, log.Unit)
	}
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
	defer logs.Unlock()
	compareLogsNoDate(c, logs.l, []appTypes.Applog{
		{Message: "Something went wrong. Check it out:", Source: "tsuru", Unit: "unit1", AppName: "fade"},
	})
}

func (s *S) TestWatchNotifySendOnClosedChannel(c *check.C) {
	svc, err := StorageAppLogService()
	c.Assert(err, check.IsNil)
	defer func() {
		c.Assert(recover(), check.IsNil)
	}()
	l, err := svc.Watch("fade", "", "")
	c.Assert(err, check.IsNil)
	l.Close()
	addLog(c, svc, "fade", "Something went wrong. Check it out:", "tsuru", "")
}
