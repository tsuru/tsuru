package registry

import (
	"time"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/log"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/scroll/vulcan/middleware"
)

// AppRegistration contains data about an app to be registered.
type AppRegistration struct {
	Name string
	Host string
	Port int
}

// HandlerRegistration contains data about a handler to be registered.
type HandlerRegistration struct {
	Name        string
	Host        string
	Path        string
	Methods     []string
	Middlewares []middleware.Middleware
}

// Registry is an interface that all built-in and user-defined registries implement.
type Registry interface {
	RegisterApp(r *AppRegistration) error
	RegisterHandler(r *HandlerRegistration) error
}

// Heartbeater periodically registers an application using the provided Registry.
type Heartbeater struct {
	Running      bool
	interval     time.Duration
	registry     Registry
	registration *AppRegistration
	ticker       *time.Ticker
	quit         chan int
}

// NewHeartbeater creates a Heartbeater from the provided app and registry.
func NewHeartbeater(registration *AppRegistration, registry Registry, interval time.Duration) *Heartbeater {
	return &Heartbeater{registration: registration, registry: registry, interval: interval}
}

// Start begins sending heartbeats.
func (h *Heartbeater) Start() {
	log.Infof("Starting heartbeat for app: %v", h.registration)

	h.Running = true
	h.ticker = time.NewTicker(h.interval)
	h.quit = make(chan int)

	go h.heartbeat()
}

// Stop halts sending heartbeats.
func (h *Heartbeater) Stop() {
	log.Infof("Stopping heartbeat for app: %v", h.registration)

	close(h.quit)
	h.ticker.Stop()
	h.Running = false
}

// Toggle starts or stops the Heartbeater based on whether it is already running.
func (h *Heartbeater) Toggle() {
	if h.Running {
		h.Stop()
	} else {
		h.Start()
	}
}

func (h *Heartbeater) heartbeat() {
	for {
		select {
		case <-h.ticker.C:
			h.registry.RegisterApp(h.registration)
		case <-h.quit:
			return
		}
	}
}
