// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/tsuru/api/shutdown"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/storage"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"github.com/tsuru/tsuru/validation"
)

var (
	_ eventTypes.WebhookService = &webhookService{}

	chanBufferSize   = 1000
	defaultUserAgent = "tsuru-webhook-client/1.0"
)

func WebhookService() (eventTypes.WebhookService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	s := &webhookService{
		storage: dbDriver.WebhookStorage,
		evtCh:   make(chan string, chanBufferSize),
		quitCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	err = s.initMetrics()
	if err != nil {
		return nil, err
	}
	go s.run()
	shutdown.Register(s)
	return s, nil
}

type webhookService struct {
	storage eventTypes.WebhookStorage
	evtCh   chan string
	quitCh  chan struct{}
	doneCh  chan struct{}

	webhooksLatency prometheus.Histogram
	webhooksTotal   prometheus.Counter
	webhooksError   prometheus.Counter
	webhooksQueue   prometheus.Collector
}

func (s *webhookService) initMetrics() error {
	s.webhooksLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "tsuru_webhooks_latency_seconds",
		Help: "The latency for webhooks requests in seconds",
	})
	s.webhooksTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_webhooks_calls_total",
		Help: "The total number of webhooks calls",
	})
	s.webhooksError = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_webhooks_calls_error",
		Help: "The total number of webhooks calls with error",
	})
	s.webhooksQueue = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsuru_webhooks_event_queue_current",
		Help: "The current number of queued events waiting for webhooks processing",
	}, func() float64 {
		return float64(len(s.evtCh))
	})
	for _, c := range []prometheus.Collector{
		s.webhooksLatency,
		s.webhooksTotal,
		s.webhooksError,
		s.webhooksQueue,
	} {
		err := prometheus.Register(c)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *webhookService) Shutdown(ctx context.Context) error {
	prometheus.Unregister(s.webhooksLatency)
	prometheus.Unregister(s.webhooksTotal)
	prometheus.Unregister(s.webhooksError)
	prometheus.Unregister(s.webhooksQueue)
	close(s.quitCh)
	select {
	case <-s.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (s *webhookService) Notify(evtID string) {
	select {
	case s.evtCh <- evtID:
	case <-s.quitCh:
	}
}

func (s *webhookService) run() {
	defer close(s.doneCh)
	for {
		select {
		case evtID := <-s.evtCh:
			err := s.handleEvent(evtID)
			if err != nil {
				log.Errorf("[webhooks] error handling webhooks for event %q: %v", evtID, err)
			}
		case <-s.quitCh:
			return
		}
	}
}

func (s *webhookService) handleEvent(evtID string) error {
	evt, err := event.GetByHexID(evtID)
	if err != nil {
		return err
	}
	filter := eventTypes.WebhookEventFilter{
		TargetTypes:  []string{string(evt.Target.Type)},
		TargetValues: []string{evt.Target.Value},
		KindTypes:    []string{string(evt.Kind.Type)},
		KindNames:    []string{evt.Kind.Name},
	}
	for _, t := range evt.ExtraTargets {
		filter.TargetTypes = append(filter.TargetTypes, string(t.Target.Type))
		filter.TargetValues = append(filter.TargetValues, t.Target.Value)
	}
	hooks, err := s.storage.FindByEvent(filter, evt.Error == "")
	if err != nil {
		return err
	}
	for _, h := range hooks {
		err = s.doHook(h, evt)
		if err != nil {
			log.Errorf("[webhooks] error calling webhook %q for event %q: %v", h.Name, evtID, err)
		}
	}
	return nil
}

func webhookBody(hook *eventTypes.Webhook, evt *event.Event) (io.Reader, error) {
	if hook.Body != "" {
		tpl, err := template.New(hook.Name).Parse(hook.Body)
		if err != nil {
			log.Errorf("[webhooks] unable to parse hook body for %q as template, using raw string: %v", hook.Name, err)
			return strings.NewReader(hook.Body), nil
		}
		buf := bytes.NewBuffer(nil)
		err = tpl.Execute(buf, evt)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(buf.Bytes()), nil
	}
	if hook.Method != http.MethodPost &&
		hook.Method != http.MethodPut &&
		hook.Method != http.MethodPatch {
		return nil, nil
	}
	hook.Headers.Set("Content-Type", "application/json")
	data, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (s *webhookService) doHook(hook eventTypes.Webhook, evt *event.Event) (err error) {
	defer func() {
		s.webhooksTotal.Inc()
		if err != nil {
			s.webhooksError.Inc()
		}
	}()
	hook.Method = strings.ToUpper(hook.Method)
	if hook.Method == "" {
		hook.Method = http.MethodPost
	}
	body, err := webhookBody(&hook, evt)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(hook.Method, hook.URL, body)
	if err != nil {
		return err
	}
	req.Header = hook.Headers
	if req.UserAgent() == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	client := tsuruNet.Dial15Full60ClientNoKeepAlive
	if hook.Insecure {
		client = tsuruNet.Dial15Full60ClientNoKeepAliveInsecure
	}
	if hook.ProxyURL != "" {
		client, err = tsuruNet.WithProxy(*client, hook.ProxyURL)
		if err != nil {
			return err
		}
	} else {
		client, err = tsuruNet.WithProxyFromConfig(*client, hook.URL)
		if err != nil {
			return err
		}
	}
	reqStart := time.Now()
	rsp, err := client.Do(req)
	s.webhooksLatency.Observe(time.Since(reqStart).Seconds())
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode < 200 || rsp.StatusCode >= 400 {
		data, _ := io.ReadAll(rsp.Body)
		return errors.Errorf("invalid status code calling hook: %d: %s", rsp.StatusCode, string(data))
	}
	return nil
}

func validateURLs(w eventTypes.Webhook) error {
	if w.URL == "" {
		return &tsuruErrors.ValidationError{Message: "webhook url must not be empty"}
	}
	_, err := url.Parse(w.URL)
	if err != nil {
		return &tsuruErrors.ValidationError{
			Message: fmt.Sprintf("webhook url is not valid: %v", err),
		}
	}
	if w.ProxyURL != "" {
		_, err = url.Parse(w.ProxyURL)
		if err != nil {
			return &tsuruErrors.ValidationError{
				Message: fmt.Sprintf("webhook proxy url is not valid: %v", err),
			}
		}
	}
	return nil
}

func (s *webhookService) Create(w eventTypes.Webhook) error {
	if w.Name == "" {
		return &tsuruErrors.ValidationError{Message: "webhook name must not be empty"}
	}
	if !validation.ValidateName(w.Name) {
		return &tsuruErrors.ValidationError{Message: "Invalid webhook name, webhook name should have at most 40 " +
			"characters, containing only lower case letters, numbers or dashes, " +
			"starting with a letter."}
	}
	err := validateURLs(w)
	if err != nil {
		return err
	}
	return s.storage.Insert(w)
}

func (s *webhookService) Update(w eventTypes.Webhook) error {
	err := validateURLs(w)
	if err != nil {
		return err
	}
	return s.storage.Update(w)
}

func (s *webhookService) Delete(name string) error {
	return s.storage.Delete(name)
}

func (s *webhookService) Find(name string) (eventTypes.Webhook, error) {
	w, err := s.storage.FindByName(name)
	if err != nil {
		return eventTypes.Webhook{}, err
	}
	return *w, nil
}

func (s *webhookService) List(teams []string) ([]eventTypes.Webhook, error) {
	return s.storage.FindAllByTeams(teams)
}
