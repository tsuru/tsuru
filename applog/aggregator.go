// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

type aggregatorLogService struct {
	base appTypes.AppLogService
}

func aggregatorAppLogService() (appTypes.AppLogService, error) {
	memory, err := memoryAppLogService()
	if err != nil {
		return nil, err
	}
	return &aggregatorLogService{
		base: memory,
	}, nil
}

func (s *aggregatorLogService) Instance() appTypes.AppLogService {
	return s.base
}

func (s *aggregatorLogService) Enqueue(entry *appTypes.Applog) error {
	return s.base.Enqueue(entry)
}

func (s *aggregatorLogService) Add(appName, message, source, unit string) error {
	return s.base.Add(appName, message, source, unit)
}

func (s *aggregatorLogService) List(args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	requests, err := buildInstanceRequests(args, false)
	if err != nil {
		return nil, errors.Wrapf(err, "[aggregator service]")
	}
	logsCh := make(chan []appTypes.Applog, len(requests))
	errCh := make(chan error, len(requests))
	wg := sync.WaitGroup{}
	for i := range requests {
		wg.Add(1)
		req := requests[i]
		go func() {
			defer wg.Done()
			logs, instanceErr := listRequest(req)
			if instanceErr != nil {
				errCh <- errors.Wrapf(instanceErr, "[log instance %v]", req.URL.Host)
				return
			}
			logsCh <- logs
		}()
	}
	wg.Wait()
	close(logsCh)
	close(errCh)
	err = <-errCh
	if err != nil {
		return nil, err
	}
	var allLogs []appTypes.Applog
	for logs := range logsCh {
		allLogs = append(allLogs, logs...)
	}
	sort.SliceStable(allLogs, func(i, j int) bool {
		return allLogs[i].Date.Before(allLogs[j].Date)
	})
	if args.Limit > 0 && len(allLogs) > args.Limit {
		allLogs = allLogs[len(allLogs)-args.Limit:]
	}
	return allLogs, nil
}

func (s *aggregatorLogService) Watch(args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	args.Limit = -1
	requests, err := buildInstanceRequests(args, true)
	if err != nil {
		return nil, errors.Wrapf(err, "[aggregator service]")
	}
	logsCh := make(chan appTypes.Applog, 1000)
	var cancels []context.CancelFunc
	for i := range requests {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		requests[i] = requests[i].WithContext(ctx)
	}
	watcher := &aggregateWatcher{channel: logsCh, cancels: cancels, quit: make(chan struct{})}
	for i := range requests {
		watcher.watchRequest(requests[i])
	}
	return watcher, nil
}

type aggregateWatcher struct {
	channel     chan appTypes.Applog
	quit        chan struct{}
	cancels     []context.CancelFunc
	wg          sync.WaitGroup
	closeCalled int32
}

func (w *aggregateWatcher) Chan() <-chan appTypes.Applog {
	return w.channel
}

func (w *aggregateWatcher) Close() {
	if atomic.AddInt32(&w.closeCalled, 1) != 1 {
		return
	}
	for _, cancel := range w.cancels {
		cancel()
	}
	close(w.quit)
	w.wg.Wait()
	close(w.channel)
}

func (w *aggregateWatcher) watchRequest(req *http.Request) {
	w.wg.Add(1)
	go func() {
		defer w.Close()
		defer w.wg.Done()
		err := w.goWatchRequest(req)
		if err != nil {
			log.Errorf("[watch log instance %v]: %v", req.URL.Host, err)
		}
	}()
}

func (w *aggregateWatcher) goWatchRequest(req *http.Request) error {
	rsp, err := tsuruNet.Dial15FullUnlimitedClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return errors.Errorf("invalid status code %v", rsp.StatusCode)
	}
	decoder := json.NewDecoder(rsp.Body)
	for {
		var logs []appTypes.Applog
		err = decoder.Decode(&logs)
		if err != nil {
			if err != io.EOF && err != context.Canceled {
				buffered, _ := ioutil.ReadAll(decoder.Buffered())
				return errors.Wrapf(err, "unable to parse as json: %q", string(buffered))
			}
			return nil
		}
		for _, log := range logs {
			select {
			case w.channel <- log:
			case <-w.quit:
				return nil
			}
		}
	}
}

func listRequest(req *http.Request) ([]appTypes.Applog, error) {
	rsp, err := tsuruNet.Dial15Full60ClientWithPool.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rsp.Body.Close()
	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("invalid status code %v: %q", rsp.StatusCode, string(data))
	}
	var logs []appTypes.Applog
	err = json.Unmarshal(data, &logs)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse log %q", string(data))
	}
	return logs, nil
}

func buildInstanceRequests(args appTypes.ListLogArgs, follow bool) ([]*http.Request, error) {
	instances, err := servicemanager.InstanceTracker.LiveInstances()
	if err != nil {
		return nil, err
	}
	var requests []*http.Request
	for _, instance := range instances {
		if len(instance.Addresses) == 0 {
			continue
		}
		ipAddr := instance.Addresses[0]
		urlValues := url.Values{}
		urlValues.Add("lines", strconv.Itoa(args.Limit))
		urlValues.Add("source", args.Source)
		for _, u := range args.Units {
			urlValues.Add("unit", u)
		}
		urlValues.Add("invert-source", strconv.FormatBool(args.InvertSource))
		if follow {
			urlValues.Add("follow", "1")
		}
		u := fmt.Sprintf("http://%s:%s/apps/%s/log-instance?%s", ipAddr, instance.Port, args.AppName, urlValues.Encode())
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		if args.Token != nil {
			req.Header.Set("Authorization", "Bearer "+args.Token.GetValue())
		}
		requests = append(requests, req)
	}
	return requests, nil
}
