// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	knet "k8s.io/apimachinery/pkg/util/net"
)

const (
	logLineTimeSeparator = " "
	logWatchBufferSize   = 1000
)

var (
	watchNewPodsInterval = time.Second * 10
	watchTimeout         = time.Hour
)

func (p *kubernetesProvisioner) ListLogs(app appTypes.App, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	clusterClient, err := clusterForPool(app.GetPool())
	if err != nil {
		return nil, err
	}
	if !clusterClient.LogsFromAPIServerEnabled() {
		return nil, provision.ErrLogsUnavailable
	}
	clusterController, err := getClusterController(p, clusterClient)
	if err != nil {
		return nil, err
	}

	ns, err := clusterClient.AppNamespace(app)
	if err != nil {
		return nil, err
	}

	podInformer, err := clusterController.getPodInformer()
	if err != nil {
		return nil, err
	}

	pods, err := podInformer.Lister().Pods(ns).List(listPodsSelectorForLog(args))
	return listLogsFromPods(clusterClient, ns, pods, args)
}

func (p *kubernetesProvisioner) WatchLogs(app appTypes.App, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	clusterClient, err := clusterForPool(app.GetPool())
	if err != nil {
		return nil, err
	}
	if !clusterClient.LogsFromAPIServerEnabled() {
		return nil, provision.ErrLogsUnavailable
	}
	clusterClient.SetTimeout(watchTimeout)

	clusterController, err := getClusterController(p, clusterClient)
	if err != nil {
		return nil, err
	}

	ns, err := clusterClient.AppNamespace(app)
	if err != nil {
		return nil, err
	}

	podInformer, err := clusterController.getPodInformer()
	if err != nil {
		return nil, err
	}

	selector := listPodsSelectorForLog(args)
	pods, err := podInformer.Lister().Pods(ns).List(selector)
	if err != nil {
		return nil, err
	}
	watchingPods := map[string]bool{}
	ctx, done := context.WithCancel(context.Background())
	watcher := &k8sLogsNotifier{
		ctx:           ctx,
		ch:            make(chan appTypes.Applog, logWatchBufferSize),
		ns:            ns,
		clusterClient: clusterClient,
		done:          done,
	}
	for _, pod := range pods {
		if pod.Status.Phase == apiv1.PodPending {
			continue
		}
		watchingPods[pod.ObjectMeta.Name] = true
		go watcher.watchPod(pod, false)
	}

	podListener := &podListener{
		OnEvent: func(pod *apiv1.Pod) {
			_, alreadyWatching := watchingPods[pod.ObjectMeta.Name]

			if !alreadyWatching && pod.Status.Phase != apiv1.PodPending {
				watchingPods[pod.ObjectMeta.Name] = true
				go watcher.watchPod(pod, true)
			}
		},
	}

	clusterController.addPodListener(args.AppName, podListener)
	go func() {
		<-ctx.Done()
		clusterController.removePodListener(args.AppName, podListener)
	}()

	return watcher, nil
}

func listLogsFromPods(clusterClient *ClusterClient, ns string, pods []*apiv1.Pod, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	var wg sync.WaitGroup
	wg.Add(len(pods))

	errs := make([]error, len(pods))
	logs := make([][]appTypes.Applog, len(pods))
	tailLimit := tailLines(args.Limit)
	if args.Limit == 0 {
		tailLimit = tailLines(100)
	}

	for index, pod := range pods {
		go func(index int, pod *apiv1.Pod) {
			defer wg.Done()

			request := clusterClient.CoreV1().Pods(ns).GetLogs(pod.ObjectMeta.Name, &apiv1.PodLogOptions{
				TailLines:  tailLimit,
				Timestamps: true,
			})
			stream, err := request.Stream()
			if err != nil {
				errs[index] = err
				return
			}

			appName := pod.ObjectMeta.Labels["tsuru.io/app-name"]
			appProcess := pod.ObjectMeta.Labels["tsuru.io/app-process"]

			scanner := bufio.NewScanner(stream)
			tsuruLogs := make([]appTypes.Applog, 0)
			for scanner.Scan() {
				line := scanner.Text()

				if len(line) == 0 {
					continue
				}
				tsuruLog := parsek8sLogLine(line)
				tsuruLog.Unit = pod.ObjectMeta.Name
				tsuruLog.AppName = appName
				tsuruLog.Source = appProcess
				tsuruLogs = append(tsuruLogs, tsuruLog)
			}

			if err := scanner.Err(); err != nil && !knet.IsProbableEOF(err) {
				errs[index] = err
			}

			logs[index] = tsuruLogs
		}(index, pod)
	}

	wg.Wait()

	unifiedLog := []appTypes.Applog{}
	for _, podLogs := range logs {
		unifiedLog = append(unifiedLog, podLogs...)
	}

	sort.Slice(unifiedLog, func(i, j int) bool { return unifiedLog[i].Date.Before(unifiedLog[j].Date) })

	for index, err := range errs {
		if err == nil {
			continue
		}

		pod := pods[index]
		appName := pod.ObjectMeta.Labels["tsuru.io/app-name"]

		unifiedLog = append(unifiedLog, errToLog(pod.ObjectMeta.Name, appName, err))
	}

	return unifiedLog, nil
}

func listPodsSelectorForLog(args appTypes.ListLogArgs) labels.Selector {
	return labels.SelectorFromSet(labels.Set(map[string]string{
		"tsuru.io/app-name": args.AppName,
	}))
}

func parsek8sLogLine(line string) (appLog appTypes.Applog) {
	parts := strings.SplitN(line, logLineTimeSeparator, 2)

	if len(parts) < 2 {
		appLog.Message = string(line)
		return
	}
	appLog.Date, _ = parseRFC3339(parts[0])
	appLog.Message = parts[1]

	return
}

func parseRFC3339(s string) (time.Time, error) {
	if t, timeErr := time.Parse(time.RFC3339Nano, s); timeErr == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func tailLines(i int) *int64 {
	b := int64(i)
	return &b
}

func errToLog(podName, appName string, err error) appTypes.Applog {
	return appTypes.Applog{
		Date:    time.Now().UTC(),
		Message: fmt.Sprintf("Could not get logs from unit: %s, error: %s", podName, err.Error()),
		Unit:    "apiserver",
		AppName: appName,
		Source:  "kubernetes",
	}
}

func infoToLog(appName string, message string) appTypes.Applog {
	return appTypes.Applog{
		Date:    time.Now().UTC(),
		Message: message,
		Unit:    "apiserver",
		AppName: appName,
		Source:  "kubernetes",
	}
}

type k8sLogsNotifier struct {
	ctx  context.Context
	ch   chan appTypes.Applog
	ns   string
	wg   sync.WaitGroup
	once sync.Once
	done context.CancelFunc

	clusterClient *ClusterClient
}

func (k *k8sLogsNotifier) watchPod(pod *apiv1.Pod, notify bool) {
	k.wg.Add(1)
	defer k.wg.Done()
	appName := pod.ObjectMeta.Labels["tsuru.io/app-name"]
	appProcess := pod.ObjectMeta.Labels["tsuru.io/app-process"]

	if notify {
		k.ch <- infoToLog(appName, "Starting to watch new unit: "+pod.ObjectMeta.Name)
	}

	var tailLines int64
	request := k.clusterClient.CoreV1().Pods(k.ns).GetLogs(pod.ObjectMeta.Name, &apiv1.PodLogOptions{
		Follow:     true,
		TailLines:  &tailLines,
		Timestamps: true,
	})
	stream, err := request.Stream()
	if err != nil {
		k.ch <- errToLog(pod.ObjectMeta.Name, appName, err)
		return
	}

	go func() {
		// TODO: after update k8s library we can put context inside the request
		<-k.ctx.Done()
		stream.Close()
	}()

	scanner := bufio.NewScanner(stream)

	for scanner.Scan() {
		line := scanner.Text()
		tsuruLog := parsek8sLogLine(line)
		tsuruLog.Unit = pod.ObjectMeta.Name
		tsuruLog.AppName = appName
		tsuruLog.Source = appProcess

		k.ch <- tsuruLog
	}

	if err := scanner.Err(); err != nil && !knet.IsProbableEOF(err) {
		k.ch <- errToLog(pod.ObjectMeta.Name, appName, err)
	}
}

func (k *k8sLogsNotifier) Chan() <-chan appTypes.Applog {
	return k.ch
}

func (k *k8sLogsNotifier) Close() {
	k.once.Do(func() {
		k.done()
		k.wg.Wait()
		close(k.ch)
	})
}
