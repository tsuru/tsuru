// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	logTypes "github.com/tsuru/tsuru/types/log"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) Test_LogsProvisioner_parsek8sLogLine(c *check.C) {
	logLine := "2020-06-18T18:47:01.885491991Z its a log line"
	tsuruLog := parsek8sLogLine(logLine)

	t, _ := time.Parse(time.RFC3339Nano, "2020-06-18T18:47:01.885491991Z")
	c.Check(tsuruLog.Date, check.Equals, t)
	c.Check(tsuruLog.Message, check.Equals, "its a log line")

	logLine = "2020-06-18T18:47:02Z its a log line"
	tsuruLog = parsek8sLogLine(logLine)

	t, _ = time.Parse(time.RFC3339, "2020-06-18T18:47:02Z")
	c.Check(tsuruLog.Date, check.Equals, t)
	c.Check(tsuruLog.Message, check.Equals, "its a log line")
}

func (s *S) Test_LogsProvisioner_ListLogs(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		tailLines, _ := strconv.Atoi(r.URL.Query().Get("tailLines"))
		for i := 1; i <= tailLines; i++ {
			fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d\n", i)
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	c.Assert(logs[0].Date.IsZero(), check.Equals, false)
	c.Assert(logs[0].Message, check.Equals, "its a message log: 1")
	c.Assert(logs[0].Source, check.Equals, "web")
	c.Assert(logs[0].Name, check.Equals, a.GetName())
	c.Assert(logs[0].Unit, check.Equals, "myapp-web-pod-1-1")
}

func (s *S) Test_LogsProvisioner_ListLongLogs(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		m := ""
		for j := 0; j <= 14; j++ {
			m = m + "long galaxy away " + m
		}
		fmt.Fprintf(w, "2019-05-06T15:04:05Z its a long message log: %s\n", m)
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Date.IsZero(), check.Equals, false)
	c.Assert(logs[0].Source, check.Equals, "web")
	c.Assert(logs[0].Name, check.Equals, a.GetName())
	c.Assert(logs[0].Unit, check.Equals, "myapp-web-pod-1-1")
}

func (s *S) Test_LogsProvisioner_ListLogsWithFilterUnits(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		tailLines, _ := strconv.Atoi(r.URL.Query().Get("tailLines"))
		for i := 1; i <= tailLines; i++ {
			fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d\n", i)
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-web-pod-1-1"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	c.Assert(logs[0].Date.IsZero(), check.Equals, false)
	c.Assert(logs[0].Message, check.Equals, "its a message log: 1")
	c.Assert(logs[0].Source, check.Equals, "web")
	c.Assert(logs[0].Name, check.Equals, a.GetName())
	c.Assert(logs[0].Unit, check.Equals, "myapp-web-pod-1-1")

	logs, err = s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-unit-not-found"},
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *S) Test_LogsProvisioner_ListLogsWithFilterSource(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		tailLines, _ := strconv.Atoi(r.URL.Query().Get("tailLines"))
		for i := 1; i <= tailLines; i++ {
			fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d\n", i)
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:   a.GetName(),
		Type:   "app",
		Limit:  10,
		Source: "web",
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
	c.Assert(logs[0].Date.IsZero(), check.Equals, false)
	c.Assert(logs[0].Message, check.Equals, "its a message log: 1")
	c.Assert(logs[0].Source, check.Equals, "web")
	c.Assert(logs[0].Name, check.Equals, a.GetName())
	c.Assert(logs[0].Unit, check.Equals, "myapp-web-pod-1-1")

	logs, err = s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:   a.GetName(),
		Type:   "app",
		Limit:  10,
		Source: "not-found",
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *S) Test_LogsProvisioner_ListLogsWithEvictedPOD(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		tailLines, _ := strconv.Atoi(r.URL.Query().Get("tailLines"))
		for i := 1; i <= tailLines; i++ {
			fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d\n", i)
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			p.Status.Phase = apiv1.PodFailed
			p.Status.Reason = "Evicted"
			_, err = s.client.CoreV1().Pods(ns).Update(context.TODO(), &p, metav1.UpdateOptions{})
			c.Assert(err, check.IsNil)
		}
	})

	logs, err := s.p.ListLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *S) Test_LogsProvisioner_WatchLogs(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		i := 0
		flusher := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d\n", i)
				flusher.Flush()
				time.Sleep(time.Second)
				i++
			}
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	logChan := watcher.Chan()

	receivedLogs := []appTypes.Applog{}

	for {
		log, ok := <-logChan
		if !ok {
			break
		}
		receivedLogs = append(receivedLogs, log)

		if len(receivedLogs) == 3 {
			watcher.Close()
		}
	}

	c.Check(receivedLogs, check.HasLen, 3)
}

func (s *S) Test_LogsProvisioner_WatchLongLogs(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		flusher := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				m := ""
				for j := 0; j <= 14; j++ {
					m = m + "long galaxy away " + m
				}
				fmt.Fprintf(w, "2019-05-06T15:04:05Z its a long message log: %s\n", m)
				flusher.Flush()
				time.Sleep(time.Second)
			}
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	logChan := watcher.Chan()

	receivedLogs := []appTypes.Applog{}

	for {
		log, ok := <-logChan
		if !ok {
			break
		}
		receivedLogs = append(receivedLogs, log)

		if len(receivedLogs) == 1 {
			watcher.Close()
		}
	}

	c.Check(receivedLogs, check.HasLen, 1)
	if !c.Check(strings.HasPrefix(receivedLogs[0].Message, "its a long message log: long galaxy away"), check.Equals, true) {
		fmt.Println("received message:", receivedLogs[0].Message)
	}
}

func (s *S) Test_LogsProvisioner_WatchLogsWithFilterUnits(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		i := 0
		flusher := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d-%s\n", i, parts[5])
				flusher.Flush()
				time.Sleep(time.Second)
				i++
			}
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-web-pod-1-1", "not-found-unit"},
	})
	c.Assert(err, check.IsNil)
	logChan := watcher.Chan()

	receivedLogs := []appTypes.Applog{}
	timer := time.After(time.Second * 5)
loop:
	for {
		select {
		case <-timer:
			break loop
		case log, ok := <-logChan:
			if !ok {
				break loop
			}
			receivedLogs = append(receivedLogs, log)

			if len(receivedLogs) == 3 {
				watcher.Close()
			}
		}
	}

	c.Check(receivedLogs, check.HasLen, 3)
}

func (s *S) Test_LogsProvisioner_WatchLogsWithEvictedUnits(c *check.C) {
	s.mock.LogHook = func(w io.Writer, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		i := 0
		flusher := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				fmt.Fprintf(w, "2019-05-06T15:04:05Z its a message log: %d-%s\n", i, parts[5])
				flusher.Flush()
				time.Sleep(time.Second)
				i++
			}
		}
	}
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(context.TODO(), &event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)
	customData := map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "run mycmd arg1",
		},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	c.Assert(err, check.IsNil)
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(len(podlist.Items), check.Equals, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			p.Status.Phase = apiv1.PodFailed
			p.Status.Reason = "Evicted"
			_, err = s.client.CoreV1().Pods(ns).Update(context.TODO(), &p, metav1.UpdateOptions{})
			c.Assert(err, check.IsNil)
		}
	})

	watcher, err := s.p.WatchLogs(context.TODO(), a, appTypes.ListLogArgs{
		Name:  a.GetName(),
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	c.Assert(err, check.IsNil)
	logChan := watcher.Chan()

	receivedLogs := []appTypes.Applog{}
	timer := time.After(time.Second * 5)
loop:
	for {
		select {
		case <-timer:
			break loop
		case log, ok := <-logChan:
			if !ok {
				break loop
			}
			receivedLogs = append(receivedLogs, log)
		}
	}

	c.Check(receivedLogs, check.HasLen, 0)
}
