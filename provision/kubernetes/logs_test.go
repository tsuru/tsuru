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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	eventTypes "github.com/tsuru/tsuru/types/event"
	logTypes "github.com/tsuru/tsuru/types/log"
	check "gopkg.in/check.v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) Test_LogsProvisioner_parsek8sLogLine(c *check.C) {
	logLine := "2020-06-18T18:47:01.885491991Z its a log line"
	tsuruLog := parsek8sLogLine(logLine)

	t, _ := time.Parse(time.RFC3339Nano, "2020-06-18T18:47:01.885491991Z")
	require.Equal(s.t, t, tsuruLog.Date)
	require.Equal(s.t, "its a log line", tsuruLog.Message)

	logLine = "2020-06-18T18:47:02Z its a log line"
	tsuruLog = parsek8sLogLine(logLine)

	t, _ = time.Parse(time.RFC3339, "2020-06-18T18:47:02Z")
	require.Equal(s.t, t, tsuruLog.Date)
	require.Equal(s.t, "its a log line", tsuruLog.Message)
}

func loggableApp(a *appTypes.App) *logTypes.LogabbleObject {
	return &logTypes.LogabbleObject{
		Name: a.Name,
		Pool: a.Pool,
	}
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 10)
	require.NotZero(s.t, logs[0].Date)
	require.Equal(s.t, "its a message log: 1", logs[0].Message)
	require.Equal(s.t, "web", logs[0].Source)
	require.Equal(s.t, a.Name, logs[0].Name)
	require.Equal(s.t, "myapp-web-pod-1-1", logs[0].Unit)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 1)
	require.NotZero(s.t, logs[0].Date)
	require.Equal(s.t, "web", logs[0].Source)
	require.Equal(s.t, a.Name, logs[0].Name)
	require.Equal(s.t, "myapp-web-pod-1-1", logs[0].Unit)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-web-pod-1-1"},
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 10)
	require.NotZero(s.t, logs[0].Date)
	require.Equal(s.t, "its a message log: 1", logs[0].Message)
	require.Equal(s.t, "web", logs[0].Source)
	require.Equal(s.t, a.Name, logs[0].Name)
	require.Equal(s.t, "myapp-web-pod-1-1", logs[0].Unit)

	logs, err = s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-unit-not-found"},
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 0)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	logs, err := s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:   a.Name,
		Type:   "app",
		Limit:  10,
		Source: "web",
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 10)
	require.NotZero(s.t, logs[0].Date)
	require.Equal(s.t, "its a message log: 1", logs[0].Message)
	require.Equal(s.t, "web", logs[0].Source)
	require.Equal(s.t, a.Name, logs[0].Name)
	require.Equal(s.t, "myapp-web-pod-1-1", logs[0].Unit)

	logs, err = s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:   a.Name,
		Type:   "app",
		Limit:  10,
		Source: "not-found",
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 0)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			p.Status.Phase = apiv1.PodFailed
			p.Status.Reason = "Evicted"
			_, err = s.client.CoreV1().Pods(ns).Update(context.TODO(), &p, metav1.UpdateOptions{})
			require.NoError(s.t, err)
		}
	})

	logs, err := s.p.ListLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
	require.Len(s.t, logs, 0)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
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

	require.Len(s.t, receivedLogs, 3)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
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

	require.Len(s.t, receivedLogs, 1)
	if !assert.True(s.t, strings.HasPrefix(receivedLogs[0].Message, "its a long message log: long galaxy away")) {
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()
	watcher, err := s.p.WatchLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
		Units: []string{"myapp-web-pod-1-1", "not-found-unit"},
	})
	require.NoError(s.t, err)
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

	require.Len(s.t, receivedLogs, 3)
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
		Target:  eventTypes.Target{Type: eventTypes.TargetTypeApp, Value: a.Name},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	require.NoError(s.t, err)
	customData := map[string][]string{
		"web": {"run", "mycmd", "arg1"},
	}
	version := newCommittedVersion(c, a, customData)
	_, err = s.p.Deploy(context.TODO(), provision.DeployArgs{App: a, Version: version, Event: evt})
	require.NoError(s.t, err)
	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	podlist, err := s.client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	require.NoError(s.t, err)
	require.Len(s.t, podlist.Items, 1)
	s.waitPodUpdate(c, func() {
		for _, p := range podlist.Items {
			p.Status.Phase = apiv1.PodFailed
			p.Status.Reason = "Evicted"
			_, err = s.client.CoreV1().Pods(ns).Update(context.TODO(), &p, metav1.UpdateOptions{})
			require.NoError(s.t, err)
		}
	})

	watcher, err := s.p.WatchLogs(context.TODO(), loggableApp(a), appTypes.ListLogArgs{
		Name:  a.Name,
		Type:  logTypes.LogTypeApp,
		Limit: 10,
	})
	require.NoError(s.t, err)
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

	require.Len(s.t, receivedLogs, 0)
}
