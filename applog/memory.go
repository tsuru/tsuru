// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package applog

import (
	"context"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	defaultMaxAppBufferSize = 1 * 1024 * 1024 // 1 MiB
	watchBufferSize         = 1000
	watchWarningInterval    = 30 * time.Second
	baseLogSize             = unsafe.Sizeof(appTypes.Applog{}) + unsafe.Sizeof(ringEntry{})
	logMemorySubsytem       = "logs_memory"
)

var (
	logsMemoryReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: logMemorySubsytem,
		Name:      "received_total",
		Help:      "The number of in memory log entries received for processing.",
	}, []string{"app"})

	logsMemoryEvicted = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: logMemorySubsytem,
		Name:      "evicted_total",
		Help:      "The number of in memory log entries removed due to full buffer.",
	}, []string{"app"})

	logsMemoryDroppedWatch = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: logMemorySubsytem,
		Name:      "watch_dropped_total",
		Help:      "The number of messages dropped in watchers due to a slow client.",
	}, []string{"app"})

	logsMemorySize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: logMemorySubsytem,
		Name:      "size",
		Help:      "The size in bytes for in memory log entries of a given app.",
	}, []string{"app"})

	logsMemoryLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: promNamespace,
		Subsystem: logMemorySubsytem,
		Name:      "length",
		Help:      "The number of in memory log entries for a given app.",
	}, []string{"app"})
)

type memoryLogService struct {
	bufferMap sync.Map
}

func memoryAppLogService() (appTypes.AppLogService, error) {
	return &memoryLogService{}, nil
}

func (s *memoryLogService) Enqueue(entry *appTypes.Applog) error {
	buffer := s.getAppBuffer(entry.AppName)
	buffer.add(entry)
	return nil
}

func (s *memoryLogService) Add(appName, message, source, unit string) error {
	messages := strings.Split(message, "\n")
	logs := make([]*appTypes.Applog, 0, len(messages))
	for _, msg := range messages {
		if msg != "" {
			l := &appTypes.Applog{
				Date:    time.Now().In(time.UTC),
				Message: msg,
				Source:  source,
				AppName: appName,
				Unit:    unit,
			}
			logs = append(logs, l)
		}
	}
	if len(logs) == 0 {
		return nil
	}
	for _, log := range logs {
		err := s.Enqueue(log)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *memoryLogService) List(ctx context.Context, args appTypes.ListLogArgs) ([]appTypes.Applog, error) {
	if args.AppName == "" {
		return nil, errors.New("app name required to list logs")
	}
	if args.Limit < 0 {
		return []appTypes.Applog{}, nil
	}
	buffer := s.getAppBuffer(args.AppName)
	if buffer.length == 0 {
		return []appTypes.Applog{}, nil
	}
	if args.Limit == 0 || buffer.length < args.Limit {
		args.Limit = buffer.length
	}
	logs := make([]appTypes.Applog, args.Limit)
	var count int
	unitsSet := set.FromSlice(args.Units)
	for current := buffer.end; count < args.Limit; {
		if (args.Source == "" || (args.Source == current.log.Source) != args.InvertSource) &&
			(len(args.Units) == 0 || unitsSet.Includes(current.log.Unit)) {

			logs[len(logs)-count-1] = *current.log
			count++
		}
		current = current.prev
		if current == buffer.end {
			break
		}
	}
	return logs[len(logs)-count:], nil
}

func (s *memoryLogService) Watch(ctx context.Context, args appTypes.ListLogArgs) (appTypes.LogWatcher, error) {
	buffer := s.getAppBuffer(args.AppName)
	watcher := &memoryWatcher{
		buffer:     buffer,
		ch:         make(chan appTypes.Applog, watchBufferSize),
		quit:       make(chan struct{}),
		wg:         &sync.WaitGroup{},
		nextNotify: time.NewTimer(0),
		filter:     args,
		unitsSet:   set.FromSlice(args.Units),
	}
	buffer.addWatcher(watcher)
	return watcher, nil

}

func (s *memoryLogService) getAppBuffer(appName string) *appLogBuffer {
	// Use a simple Load first to avoid unnecessary allocations and the common
	// case is Load being successful.
	buffer, ok := s.bufferMap.Load(appName)
	if !ok {
		buffer, _ = s.bufferMap.LoadOrStore(appName, &appLogBuffer{
			appName:         appName,
			receivedCounter: logsMemoryReceived.WithLabelValues(appName),
			evictedCounter:  logsMemoryEvicted.WithLabelValues(appName),
			droppedCounter:  logsMemoryDroppedWatch.WithLabelValues(appName),
			sizeGauge:       logsMemorySize.WithLabelValues(appName),
			lengthGauge:     logsMemoryLength.WithLabelValues(appName),
			bufferMaxSize:   appBufferSize(),
		})
	}
	return buffer.(*appLogBuffer)
}

func appBufferSize() uint {
	bufferSize, _ := config.GetUint("log:app-log-memory-buffer-bytes")
	if bufferSize == 0 {
		bufferSize = defaultMaxAppBufferSize
	}
	return bufferSize
}

type ringEntry struct {
	log        *appTypes.Applog
	size       uint
	next, prev *ringEntry
}

type appLogBuffer struct {
	mu              sync.Mutex
	appName         string
	size            uint
	length          int
	bufferMaxSize   uint
	start, end      *ringEntry
	watchers        []*memoryWatcher
	receivedCounter prometheus.Counter
	evictedCounter  prometheus.Counter
	droppedCounter  prometheus.Counter
	sizeGauge       prometheus.Gauge
	lengthGauge     prometheus.Gauge
}

func (b *appLogBuffer) add(entry *appTypes.Applog) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.receivedCounter.Inc()
	next := &ringEntry{
		log:  entry,
		size: entrySize(entry),
	}
	if next.size > b.bufferMaxSize {
		return
	}
	if b.start == nil {
		b.start = next
		b.end = next
	}
	next.next = b.start
	next.prev = b.end
	b.start.prev = next
	b.end.next = next
	b.end = b.end.next
	b.length++
	newFullSize := b.size + next.size
	for newFullSize > b.bufferMaxSize {
		newFullSize -= b.start.size
		b.start = b.start.next
		b.start.prev = b.end
		b.end.next = b.start
		b.length--
		b.evictedCounter.Inc()
	}
	b.size = newFullSize
	b.sizeGauge.Set(float64(b.size))
	b.lengthGauge.Set(float64(b.length))
	for _, w := range b.watchers {
		w.notify(entry, b.droppedCounter)
	}
}

func (b *appLogBuffer) addWatcher(watcher *memoryWatcher) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.watchers = append(b.watchers, watcher)
}

func (b *appLogBuffer) removeWatcher(watcher *memoryWatcher) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.watchers {
		if b.watchers[i] == watcher {
			b.watchers[i] = b.watchers[len(b.watchers)-1]
			b.watchers = b.watchers[:len(b.watchers)-1]
			return true
		}
	}
	return false
}

func entrySize(entry *appTypes.Applog) uint {
	return uint(len(entry.AppName) +
		len(entry.Message) +
		len(entry.MongoID) +
		len(entry.Source) +
		len(entry.Unit) +
		int(baseLogSize))
}

type memoryWatcher struct {
	buffer     *appLogBuffer
	ch         chan appTypes.Applog
	quit       chan struct{}
	wg         *sync.WaitGroup
	nextNotify *time.Timer
	filter     appTypes.ListLogArgs
	unitsSet   set.Set
}

func (w *memoryWatcher) notify(entry *appTypes.Applog, dropCounter prometheus.Counter) {
	if w.filter.Source != "" && ((w.filter.Source != entry.Source) != w.filter.InvertSource) {
		return
	}
	if len(w.filter.Units) > 0 && !w.unitsSet.Includes(entry.Unit) {
		return
	}
	select {
	case w.ch <- *entry:
	default:
		dropCounter.Inc()
		select {
		case <-w.nextNotify.C:
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				select {
				case w.ch <- slowWatcherWarning(entry.AppName):
				case <-w.quit:
				}
				w.nextNotify.Reset(watchWarningInterval)
			}()
		default:
		}
	}
}

func (w *memoryWatcher) Chan() <-chan appTypes.Applog {
	return w.ch
}

func (w *memoryWatcher) Close() {
	if w.buffer.removeWatcher(w) {
		close(w.quit)
		w.wg.Wait()
		close(w.ch)
	}
}

func slowWatcherWarning(appName string) appTypes.Applog {
	return appTypes.Applog{
		AppName: appName,
		Date:    time.Now(),
		Message: "Log messages dropped due to slow tail client or too many messages being produced.",
		Source:  "tsuru",
		Unit:    "api",
	}
}
