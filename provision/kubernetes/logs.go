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

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	logTypes "github.com/tsuru/tsuru/types/log"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	knet "k8s.io/apimachinery/pkg/util/net"
)

const (
	logLineTimeSeparator = " "
	logWatchBufferSize   = 1000
)

var watchTimeout = time.Hour

func (p *kubernetesProvisioner) ListLogs(ctx context.Context, obj *logTypes.LogabbleObject, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	clusterClient, err := clusterForPool(ctx, obj.Pool)
	if err != nil {
		return nil, err
	}
	clusterController, err := getClusterController(p, clusterClient)
	if err != nil {
		return nil, err
	}

	ns := clusterClient.PoolNamespace(obj.Pool)

	podInformer, err := clusterController.getPodInformer()
	if err != nil {
		return nil, err
	}

	pods, err := podInformer.Lister().Pods(ns).List(listPodsSelectorForLog(args))
	if err != nil {
		return nil, err
	}
	if len(args.Units) > 0 {
		pods = filterPods(pods, args.Units)
	}
	return listLogsFromPods(ctx, clusterClient, ns, pods, args)
}

func (p *kubernetesProvisioner) WatchLogs(ctx context.Context, obj *logTypes.LogabbleObject, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	pool := obj.Pool
	clusterClient, err := clusterForPool(ctx, pool)
	if err != nil {
		return nil, err
	}
	clusterClient.SetTimeout(watchTimeout)

	clusterController, err := getClusterController(p, clusterClient)
	if err != nil {
		return nil, err
	}

	ns := clusterClient.PoolNamespace(pool)

	podInformer, err := clusterController.getPodInformer()
	if err != nil {
		return nil, err
	}

	selector := listPodsSelectorForLog(args)
	pods, err := podInformer.Lister().Pods(ns).List(selector)
	if err != nil {
		return nil, err
	}
	if len(args.Units) > 0 {
		pods = filterPods(pods, args.Units)
	}
	uuidV4, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate uuid v4")
	}
	ctx, done := context.WithCancel(context.Background())
	watcher := &k8sLogsWatcher{
		id:           uuidV4.String(),
		logArgs:      args,
		ctx:          ctx,
		ch:           make(chan appTypes.Applog, logWatchBufferSize),
		ns:           ns,
		done:         done,
		watchingPods: map[string]bool{},

		clusterClient:     clusterClient,
		clusterController: clusterController,
	}
	for _, pod := range pods {
		if !loggablePod(&pod.Status) {
			continue
		}
		watcher.watchingPods[pod.ObjectMeta.Name] = true
		watcher.wg.Add(1)
		go watcher.watchPod(pod, false)
	}

	clusterController.addPodListener(watcher.id, watcher)

	return watcher, nil
}

func listLogsFromPods(ctx context.Context, clusterClient *ClusterClient, ns string, pods []*apiv1.Pod, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	var wg sync.WaitGroup

	errs := make([]error, len(pods))
	logs := make([][]appTypes.Applog, len(pods))
	tailLimit := tailLines(args.Limit)
	if args.Limit == 0 {
		tailLimit = tailLines(100)
	}

	for index, pod := range pods {
		if !loggablePod(&pod.Status) {
			continue
		}
		wg.Add(1)
		go func(index int, pod *apiv1.Pod) {
			defer wg.Done()

			request := clusterClient.CoreV1().Pods(ns).GetLogs(pod.ObjectMeta.Name, &apiv1.PodLogOptions{
				TailLines:  tailLimit,
				Timestamps: true,
			})
			stream, err := request.Stream(ctx)
			if err != nil {
				errs[index] = err
				return
			}

			name, logType := logMetadata(pod.ObjectMeta.Labels)
			appProcess := pod.ObjectMeta.Labels[tsuruLabelAppProcess]

			reader := bufio.NewReader(stream)
			tsuruLogs := make([]appTypes.Applog, 0)

			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if !knet.IsProbableEOF(err) {
						errs[index] = err
					}
					break
				}

				if len(line) == 0 {
					continue
				}

				tsuruLog := parsek8sLogLine(strings.TrimSpace(string(line)))
				tsuruLog.Unit = pod.ObjectMeta.Name
				tsuruLog.Name = name
				tsuruLog.Type = logType
				tsuruLog.Source = appProcess
				tsuruLogs = append(tsuruLogs, tsuruLog)
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
		appName := pod.ObjectMeta.Labels[tsuruLabelAppName]

		unifiedLog = append(unifiedLog, errToLog(pod.ObjectMeta.Name, appName, err))
	}

	return unifiedLog, nil
}

func listPodsSelectorForLog(args appTypes.ListLogArgs) labels.Selector {
	m := map[string]string{}
	if args.Type == logTypes.LogTypeJob {
		m[tsuruLabelJobName] = args.Name
	} else {
		m[tsuruLabelAppName] = args.Name
	}
	if args.Source != "" {
		m[tsuruLabelAppProcess] = args.Source
	}
	return labels.SelectorFromSet(labels.Set(m))
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
		Name:    appName,
		Source:  "kubernetes",
	}
}

func infoToLog(appName string, message string) appTypes.Applog {
	return appTypes.Applog{
		Date:    time.Now().UTC(),
		Message: message,
		Unit:    "apiserver",
		Name:    appName,
		Source:  "kubernetes",
	}
}

type k8sLogsWatcher struct {
	id   string
	ctx  context.Context
	ch   chan appTypes.Applog
	ns   string
	wg   sync.WaitGroup
	once sync.Once
	done context.CancelFunc

	logArgs           appTypes.ListLogArgs
	clusterClient     *ClusterClient
	clusterController *clusterController
	watchingPods      map[string]bool
}

func logMetadata(labels map[string]string) (string, logTypes.LogType) {
	if appName, ok := labels[tsuruLabelAppName]; ok {
		return appName, logTypes.LogTypeApp
	} else if jobName, ok := labels[tsuruLabelJobName]; ok {
		return jobName, logTypes.LogTypeJob
	}
	return "", logTypes.LogTypeApp
}

func (k *k8sLogsWatcher) watchPod(pod *apiv1.Pod, addedLater bool) {
	defer k.wg.Done()
	name, logType := logMetadata(pod.ObjectMeta.Labels)
	appProcess := pod.ObjectMeta.Labels[tsuruLabelAppProcess]
	var tailLines int64

	if addedLater {
		k.ch <- infoToLog(name, "Starting to watch new unit: "+pod.ObjectMeta.Name)
		tailLines = int64(k.logArgs.Limit) // shun that startup logs be forgotten
	}

	request := k.clusterClient.CoreV1().Pods(k.ns).GetLogs(pod.ObjectMeta.Name, &apiv1.PodLogOptions{
		Follow:     true,
		TailLines:  &tailLines,
		Timestamps: true,
	})
	stream, err := request.Stream(k.ctx)
	if err != nil {
		k.ch <- errToLog(pod.ObjectMeta.Name, name, err)
		return
	}

	reader := bufio.NewReader(stream)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if !knet.IsProbableEOF(err) && err != context.Canceled {
				k.ch <- errToLog(pod.ObjectMeta.Name, name, err)
			}
			break
		}

		if len(line) == 0 {
			continue
		}

		tsuruLog := parsek8sLogLine(strings.TrimSpace(string(line)))
		tsuruLog.Unit = pod.ObjectMeta.Name
		tsuruLog.Name = name
		tsuruLog.Type = logType
		tsuruLog.Source = appProcess
		k.ch <- tsuruLog
	}
}

func (k *k8sLogsWatcher) Chan() <-chan appTypes.Applog {
	return k.ch
}

func (k *k8sLogsWatcher) Close() {
	k.once.Do(func() {
		k.clusterController.removePodListener(k.id)
		k.done()
		k.wg.Wait()
		close(k.ch)
	})
}

func (k *k8sLogsWatcher) OnPodEvent(pod *apiv1.Pod) {
	appName := pod.ObjectMeta.Labels[tsuruLabelAppName]
	jobName := pod.ObjectMeta.Labels[tsuruLabelJobName]
	if k.logArgs.Name != appName && k.logArgs.Name != jobName {
		return
	}

	_, alreadyWatching := k.watchingPods[pod.ObjectMeta.Name]
	podMatches := matchPod(pod, k.logArgs)
	if !alreadyWatching && podMatches && loggablePod(&pod.Status) {
		k.watchingPods[pod.ObjectMeta.Name] = true
		k.wg.Add(1)
		go k.watchPod(pod, true)
	}
}

func filterPods(pods []*apiv1.Pod, names []string) []*apiv1.Pod {
	nameSet := set.FromSlice(names)
	result := []*apiv1.Pod{}
	for _, pod := range pods {
		if nameSet.Includes(pod.ObjectMeta.Name) {
			result = append(result, pod)
		}
	}

	return result
}

func matchPod(pod *apiv1.Pod, args appTypes.ListLogArgs) bool {
	if pod.ObjectMeta.Labels[tsuruLabelIsBuild] == "true" {
		return false
	}
	if args.Source != "" && pod.ObjectMeta.Labels[tsuruLabelAppProcess] != args.Source {
		return false
	}

	if len(args.Units) > 0 {
		nameSet := set.FromSlice(args.Units)
		if !nameSet.Includes(pod.ObjectMeta.Name) {
			return false
		}
	}

	return true
}

func loggablePod(podStatus *apiv1.PodStatus) bool {
	if podStatus.Phase == apiv1.PodFailed && podStatus.Reason == "Evicted" {
		return false
	}

	return podStatus.Phase != apiv1.PodPending
}
