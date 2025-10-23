// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracker

import (
	"context"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/shutdown"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/storage"
	trackerTypes "github.com/tsuru/tsuru/types/tracker"
	"go.opentelemetry.io/otel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultUpdateInterval = 15 * time.Second
	defaultStaleTimeout   = 50 * time.Second
)

var (
	_ trackerTypes.InstanceService = &instanceTracker{}
	_ trackerTypes.InstanceService = &k8sInstanceTracker{}
)

func InstanceService() (trackerTypes.InstanceService, error) {
	useK8s, _ := config.GetBool("tracker:use-kubernetes")

	if useK8s {
		return newK8sInstanceTracker()
	}

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
	storage      trackerTypes.InstanceStorage
	quit         chan struct{}
	done         chan struct{}
	mu           sync.Mutex
	lastInstance *trackerTypes.TrackedInstance
}

func (t *instanceTracker) start() {
	defer close(t.done)
	tracer := otel.Tracer("tsuru/api/tracker")
	for {
		ctx, span := tracer.Start(context.Background(), "InstanceTracker notify")
		err := t.notify(ctx)
		if err != nil {
			log.Errorf("[instance-tracker] unable to track instance: %v", err)
		}
		span.End()

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

func (t *instanceTracker) notify(ctx context.Context) error {
	instance, err := t.getInstance(true)
	if err != nil {
		return err
	}
	return t.storage.Notify(ctx, instance)
}

func (t *instanceTracker) getInstance(update bool) (trackerTypes.TrackedInstance, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if update || t.lastInstance == nil {
		instance, err := createInstance()
		if err != nil {
			return instance, err
		}
		t.lastInstance = &instance
	}
	return *t.lastInstance, nil
}

func createInstance() (trackerTypes.TrackedInstance, error) {
	var instance trackerTypes.TrackedInstance
	iface, err := getInterface()
	if err != nil {
		return instance, err
	}
	ipv4Only, err := config.GetBool("tracker:ipv4-only")
	if err != nil {
		ipv4Only = true
	}
	port, tlsPort, err := listenPorts()
	if err != nil {
		return instance, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return instance, err
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return instance, err
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
	return trackerTypes.TrackedInstance{
		Name:       hostname,
		Port:       port,
		TLSPort:    tlsPort,
		Addresses:  ips,
		LastUpdate: time.Now().UTC().Truncate(time.Millisecond),
	}, nil
}

func listenPorts() (port string, tlsPort string, err error) {
	tlsListen, _ := config.GetString("tls:listen")
	if tlsListen != "" {
		_, tlsPort, err = net.SplitHostPort(tlsListen)
		if err != nil {
			return "", "", err
		}
	}
	listen, _ := config.GetString("listen")
	if listen != "" {
		_, port, err = net.SplitHostPort(listen)
		if err != nil {
			return "", "", err
		}
	}

	return port, tlsPort, nil
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

func (t *instanceTracker) CurrentInstance(ctx context.Context) (trackerTypes.TrackedInstance, error) {
	return t.getInstance(false)
}

func (t *instanceTracker) LiveInstances(ctx context.Context) ([]trackerTypes.TrackedInstance, error) {
	var staleTimeout time.Duration
	staleTimeoutSeconds, _ := config.GetFloat("tracker:stale-timeout")
	if staleTimeoutSeconds != 0 {
		staleTimeout = time.Duration(staleTimeoutSeconds * float64(time.Second))
	} else {
		staleTimeout = defaultStaleTimeout
	}
	return t.storage.List(ctx, staleTimeout)
}

func getInterface() (net.Interface, error) {
	interfaceName, _ := config.GetString("tracker:interface")
	var interfaceNames []string
	if interfaceName == "" {
		interfaceNames = []string{"eth0", "en0"}
	} else {
		interfaceNames = []string{interfaceName}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	if len(ifaces) == 0 {
		return net.Interface{}, errors.New("no network interfaces available")
	}
	for _, wanted := range interfaceNames {
		for _, iface := range ifaces {
			if iface.Name == wanted {
				return iface, nil
			}
		}
	}
	if interfaceName != "" {
		return net.Interface{}, errors.Errorf("interface named %q not found", interfaceName)
	}
	return ifaces[0], nil
}

// instanceTracker based on kubernetes service discovery
type k8sInstanceTracker struct {
	ns              string
	service         string
	cli             kubernetes.Interface
	currentInstance trackerTypes.TrackedInstance
}

func (t *k8sInstanceTracker) CurrentInstance(ctx context.Context) (trackerTypes.TrackedInstance, error) {
	return t.currentInstance, nil
}

func (t *k8sInstanceTracker) LiveInstances(ctx context.Context) ([]trackerTypes.TrackedInstance, error) {
	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		"kubernetes.io/service-name": t.service,
	}))

	endpointSlices, err := t.cli.DiscoveryV1().EndpointSlices(t.ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	instances := []trackerTypes.TrackedInstance{}
	lastUpdate := time.Now()

	defaultHTTPPort, defaultHTTPSPort, err := listenPorts()
	if err != nil {
		return nil, err
	}
	if defaultHTTPPort == "" {
		defaultHTTPPort = "80"
	}
	if defaultHTTPSPort == "" {
		defaultHTTPSPort = "443"
	}

	for _, endpointSlice := range endpointSlices.Items {
		httpPort := defaultHTTPPort
		httpsPort := defaultHTTPSPort

		for _, port := range endpointSlice.Ports {
			if port.Port == nil || port.Name == nil {
				continue
			}

			if *port.Name == "http" {
				httpPort = strconv.Itoa(int(*port.Port))
			}

			if *port.Name == "https" {
				httpsPort = strconv.Itoa(int(*port.Port))
			}
		}

		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.TargetRef == nil {
				continue
			}
			instances = append(instances, trackerTypes.TrackedInstance{
				Name:       endpoint.TargetRef.Name,
				Addresses:  endpoint.Addresses,
				Port:       httpPort,
				TLSPort:    httpsPort,
				LastUpdate: lastUpdate,
			})
		}
	}

	return instances, nil
}

func newK8sInstanceTracker() (*k8sInstanceTracker, error) {
	inClusterCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	cli, err := kubernetes.NewForConfig(inClusterCfg)
	if err != nil {
		return nil, err
	}

	ns, err := config.GetString("tracker:kubernetes-namespace")
	if err != nil {
		return nil, err
	}

	service, err := config.GetString("tracker:kubernetes-service")
	if err != nil {
		return nil, err
	}

	currentInstance, err := createInstance()
	if err != nil {
		return nil, err
	}

	return &k8sInstanceTracker{
		cli:             cli,
		ns:              ns,
		service:         service,
		currentInstance: currentInstance,
	}, nil
}
