// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	trackerTypes "github.com/tsuru/tsuru/types/tracker"
)

const (
	defaultUpdateInterval = 15 * time.Second
	defaultStaleTimeout   = 50 * time.Second
	defaultInterfaceName  = "eth0"
)

var _ trackerTypes.InstanceService = &instanceTracker{}

func InstanceService() (trackerTypes.InstanceService, error) {
	dbDriver, err := storage.GetCurrentDbDriver()
	if err != nil {
		dbDriver, err = storage.GetDefaultDbDriver()
		if err != nil {
			return nil, err
		}
	}
	tracker := &instanceTracker{
		storage: dbDriver.InstanceTrackerStorage,
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go tracker.start()
	shutdown.Register(tracker)
	return tracker, nil
}

type instanceTracker struct {
	storage trackerTypes.InstanceStorage
	quit    chan struct{}
	done    chan struct{}
}

func (t *instanceTracker) start() {
	defer close(t.done)
	for {
		err := t.notify()
		if err != nil {
			log.Errorf("[instance-tracker] unable to track instance: %v", err)
		}

		var updateInterval time.Duration
		updateIntervalSeconds, _ := config.GetFloat("tracker:update-interval")
		if updateIntervalSeconds != 0 {
			updateInterval = time.Duration(updateIntervalSeconds * float64(time.Second))
		} else {
			updateInterval = defaultUpdateInterval
		}
		select {
		case <-t.quit:
			return
		case <-time.After(updateInterval):
		}
	}
}

func (t *instanceTracker) notify() error {
	interfaceName, _ := config.GetString("tracker:interface")
	if interfaceName == "" {
		interfaceName = defaultInterfaceName
	}
	ipv4Only, err := config.GetBool("tracker:ipv4-only")
	if err != nil {
		ipv4Only = true
	}
	var port, tlsPort string
	tlsListen, _ := config.GetString("tls:listen")
	if tlsListen != "" {
		_, tlsPort, err = net.SplitHostPort(tlsListen)
		if err != nil {
			return err
		}
	}
	listen, _ := config.GetString("listen")
	if listen != "" {
		_, port, err = net.SplitHostPort(listen)
		if err != nil {
			return err
		}
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return err
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return err
	}
	ips := make([]string, 0, len(addresses))
	for _, ifaceAddr := range addresses {
		if !ipv4Only {
			ips = append(ips, ifaceAddr.String())
			continue
		}
		if ipNet, ok := ifaceAddr.(*net.IPNet); ok {
			ipv4 := ipNet.IP.To4()
			if ipv4 != nil {
				ips = append(ips, ipv4.String())
			}
		}
	}
	instance := trackerTypes.TrackedInstance{
		Name:      hostname,
		Port:      port,
		TLSPort:   tlsPort,
		Addresses: ips,
	}
	return t.storage.Notify(instance)
}

func (t *instanceTracker) Shutdown(ctx context.Context) error {
	close(t.quit)
	select {
	case <-t.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (t *instanceTracker) LiveInstances() ([]trackerTypes.TrackedInstance, error) {
	var staleTimeout time.Duration
	staleTimeoutSeconds, _ := config.GetFloat("tracker:stale-timeout")
	if staleTimeoutSeconds != 0 {
		staleTimeout = time.Duration(staleTimeoutSeconds * float64(time.Second))
	} else {
		staleTimeout = defaultStaleTimeout
	}
	return t.storage.List(staleTimeout)
}
