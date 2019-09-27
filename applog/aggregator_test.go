// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/tracker"
	"gopkg.in/check.v1"
)

type mockInstanceService struct {
	instances []tracker.TrackedInstance
}

func (m *mockInstanceService) LiveInstances() ([]tracker.TrackedInstance, error) {
	return m.instances, nil
}

func (m *mockInstanceService) CurrentInstance() (tracker.TrackedInstance, error) {
	return tracker.TrackedInstance{}, nil
}

func mockServers(count int, hook func(i int, w http.ResponseWriter, r *http.Request) bool) func() {
	instanceTracker := &mockInstanceService{}
	srvs := make([]*httptest.Server, count)
	for i := range srvs {
		i := i
		ts := time.Now().Add(time.Duration(i) * time.Second)
		response := []appTypes.Applog{
			{Message: fmt.Sprintf("msg%d", i), Date: ts},
		}
		srvs[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hook != nil {
				if hook(i, w, r) {
					return
				}
			}
			json.NewEncoder(w).Encode(response)
		}))
		u, _ := url.Parse(srvs[i].URL)
		host, port, _ := net.SplitHostPort(u.Host)
		instanceTracker.instances = append(instanceTracker.instances, tracker.TrackedInstance{
			Addresses: []string{host}, Port: port,
		})
	}
	servicemanager.InstanceTracker = instanceTracker
	return func() {
		for _, srv := range srvs {
			srv.Close()
		}
	}
}

func (s *S) Test_Aggregator_List(c *check.C) {
	rollback := mockServers(5, func(i int, w http.ResponseWriter, r *http.Request) bool {
		c.Assert(r.URL.Path, check.Equals, "/apps/myapp/log-instance")
		c.Assert(r.URL.Query().Get("lines"), check.Equals, "0")
		c.Assert(r.URL.Query().Get("source"), check.Equals, "")
		c.Assert(r.URL.Query().Get("unit"), check.Equals, "")
		c.Assert(r.URL.Query().Get("invert-filters"), check.Equals, "false")
		c.Assert(r.URL.Query().Get("follow"), check.Equals, "")
		return false
	})
	defer rollback()
	svc := &aggregatorLogService{}
	logs, err := svc.List(appTypes.ListLogArgs{
		AppName: "myapp",
	})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, logs, []appTypes.Applog{
		{Message: "msg0"},
		{Message: "msg1"},
		{Message: "msg2"},
		{Message: "msg3"},
		{Message: "msg4"},
	})
}

func (s *S) Test_Aggregator_ListFilter(c *check.C) {
	rollback := mockServers(5, func(i int, w http.ResponseWriter, r *http.Request) bool {
		c.Assert(r.URL.Path, check.Equals, "/apps/myapp/log-instance")
		c.Assert(r.URL.Query().Get("lines"), check.Equals, "10")
		c.Assert(r.URL.Query().Get("source"), check.Equals, "tsuru")
		c.Assert(r.URL.Query().Get("unit"), check.Equals, "myunit")
		c.Assert(r.URL.Query().Get("invert-filters"), check.Equals, "true")
		c.Assert(r.URL.Query().Get("follow"), check.Equals, "")
		return false
	})
	defer rollback()
	svc := &aggregatorLogService{}
	logs, err := svc.List(appTypes.ListLogArgs{
		AppName:       "myapp",
		Source:        "tsuru",
		Unit:          "myunit",
		InvertFilters: true,
		Limit:         10,
	})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, logs, []appTypes.Applog{
		{Message: "msg0"},
		{Message: "msg1"},
		{Message: "msg2"},
		{Message: "msg3"},
		{Message: "msg4"},
	})
}

func (s *S) Test_Aggregator_ListReorderMessages(c *check.C) {
	rollback := mockServers(6, func(i int, w http.ResponseWriter, r *http.Request) bool {
		switch i {
		case 0:
			w.Write([]byte(`[{
				"Date": "2019-07-23T10:51:20-03:00",
				"Message": "msg1",
				"Source": "web",
				"AppName": "myapp",
				"Unit": "unit1"
			}]`))
		case 1:
			w.Write([]byte(`[{
				"Date": "2019-07-23T13:50:26.634644022Z",
				"Message": "msg2",
				"Source": "tsuru",
				"AppName": "myapp",
				"Unit": "api"
			}]`))
		case 2:
			w.Write([]byte(`[{
				"Date": "2019-07-23T12:57:47.197062857Z",
				"Message": "msg3",
				"Source": "tsuru",
				"AppName": "myapp",
				"Unit": "api"
			}]`))
		case 3:
			w.Write([]byte(`[{
				"Date": "2019-07-23T13:21:44.675499702Z",
				"Message": "msg4",
				"Source": "tsuru",
				"AppName": "myapp",
				"Unit": "api"
			}]`))
		case 4:
			w.Write([]byte(`[{
				"Date": "2019-07-23T13:08:16.014858022Z",
				"Message": "msg5",
				"Source": "tsuru",
				"AppName": "myapp",
				"Unit": "api"
			}]`))
		case 5:
			w.Write([]byte(`[]`))
		}
		return true
	})
	defer rollback()
	svc := &aggregatorLogService{}
	logs, err := svc.List(appTypes.ListLogArgs{
		AppName: "myapp",
		Limit:   1,
	})
	c.Assert(err, check.IsNil)
	compareLogsNoDate(c, logs, []appTypes.Applog{{
		Message: "msg1",
		Source:  "web",
		AppName: "myapp",
		Unit:    "unit1",
	}})
}

func (s *S) Test_Aggregator_List_WithError(c *check.C) {
	rollback := mockServers(5, func(i int, w http.ResponseWriter, r *http.Request) bool {
		w.WriteHeader(http.StatusInternalServerError)
		return true
	})
	defer rollback()
	svc := &aggregatorLogService{}
	_, err := svc.List(appTypes.ListLogArgs{
		AppName: "myapp",
	})
	c.Assert(err, check.ErrorMatches, `(?s)\[log instance .*\]: invalid status code 500:.*`)
}

func (s *S) Test_Aggregator_List_WithErrorSingleRequest(c *check.C) {
	rollback := mockServers(5, func(i int, w http.ResponseWriter, r *http.Request) bool {
		if i == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return false
	})
	defer rollback()
	svc := &aggregatorLogService{}
	_, err := svc.List(appTypes.ListLogArgs{
		AppName: "myapp",
	})
	c.Assert(err, check.ErrorMatches, `(?s)\[log instance .*\]: invalid status code 500:.*`)
}

func (s *S) Test_Aggregator_Watch(c *check.C) {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	ch3 := make(chan struct{})
	rollback := mockServers(2, func(i int, w http.ResponseWriter, r *http.Request) bool {
		enc := json.NewEncoder(w)
		response := []appTypes.Applog{{Message: "msg-0"}}
		enc.Encode(response)
		w.(http.Flusher).Flush()
		<-ch1
		response = []appTypes.Applog{{Message: "msg-1"}}
		enc.Encode(response)
		w.(http.Flusher).Flush()
		<-ch2
		response = []appTypes.Applog{{Message: "msg-2"}}
		enc.Encode(response)
		w.(http.Flusher).Flush()
		<-ch3
		return true
	})
	_ = rollback
	defer rollback()
	svc := &aggregatorLogService{}
	watcher, err := svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	defer watcher.Close()
	ch := watcher.Chan()
	c.Check(msgTimeout(c, ch), check.Equals, "msg-0")
	c.Check(msgTimeout(c, ch), check.Equals, "msg-0")
	close(ch1)
	c.Check(msgTimeout(c, ch), check.Equals, "msg-1")
	c.Check(msgTimeout(c, ch), check.Equals, "msg-1")
	close(ch2)
	c.Check(msgTimeout(c, ch), check.Equals, "msg-2")
	c.Check(msgTimeout(c, ch), check.Equals, "msg-2")
	close(ch3)
	var msg appTypes.Applog
	var isOpen bool
	select {
	case msg, isOpen = <-ch:
	case <-time.After(5 * time.Second):
		c.Fatal("timeout waiting for channel close")
	}
	c.Assert(msg, check.DeepEquals, appTypes.Applog{})
	c.Assert(isOpen, check.Equals, false)
}

func (s *S) Test_Aggregator_Watch_WithErrorAfterMessages(c *check.C) {
	allServersWG := sync.WaitGroup{}
	allServersWG.Add(3)
	okServersWG := sync.WaitGroup{}
	okServersWG.Add(2)
	rollback := mockServers(3, func(i int, w http.ResponseWriter, r *http.Request) bool {
		defer allServersWG.Done()
		if i == 1 {
			okServersWG.Wait()
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		enc := json.NewEncoder(w)
		notified := false
		for {
			response := []appTypes.Applog{{Message: "msg"}}
			err := enc.Encode(response)
			if err != nil {
				break
			}
			w.(http.Flusher).Flush()
			if !notified {
				notified = true
				okServersWG.Done()
			}
			time.Sleep(100 * time.Millisecond)
		}
		return true
	})
	_ = rollback
	defer rollback()
	svc := &aggregatorLogService{}
	watcher, err := svc.Watch("myapp", "", "", nil)
	c.Assert(err, check.IsNil)
	defer watcher.Close()
	doneCh := make(chan struct{})
	go func() {
		allServersWG.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		c.Error("timeout waiting for servers to finish after error in one of them")
	}
}

func msgTimeout(c *check.C, ch <-chan appTypes.Applog) string {
	select {
	case v := <-ch:
		return v.Message
	case <-time.After(2 * time.Second):
		c.Error("timeout waiting log")
	}
	return ""
}
