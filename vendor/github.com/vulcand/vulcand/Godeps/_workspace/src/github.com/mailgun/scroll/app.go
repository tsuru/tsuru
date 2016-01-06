package scroll

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/gorilla/mux"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/log"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/manners"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/metrics"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/scroll/registry"
	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/scroll/vulcan/middleware"
)

const (
	// Suggested result set limit for APIs that may return many entries (e.g. paging).
	DefaultLimit = 100

	// Suggested max allowed result set limit for APIs that may return many entries (e.g. paging).
	MaxLimit = 10000

	// Suggested max allowed amount of entries that batch APIs can accept (e.g. batch uploads).
	MaxBatchSize = 1000

	// Interval between Vulcand heartbeats (if the app if configured to register in it).
	defaultRegisterInterval = 2 * time.Second
)

// Represents an app.
type App struct {
	Config      AppConfig
	router      *mux.Router
	stats       *appStats
	heartbeater *registry.Heartbeater
}

// Represents a configuration object an app is created with.
type AppConfig struct {
	// name of the app being created
	Name string

	// IP/port the app will bind to
	ListenIP   string
	ListenPort int

	// optional router to use
	Router *mux.Router

	// hostnames of the public and protected API entrypoints used for vulcan registration
	PublicAPIHost    string
	ProtectedAPIHost string
	ProtectedAPIURL  string

	// how to register the app's endpoint and handlers in vulcan
	Registry registry.Registry
	Interval time.Duration

	// metrics service used for emitting the app's real-time metrics
	Client metrics.Client
}

// Create a new app.
func NewApp() *App {
	return NewAppWithConfig(AppConfig{})
}

// Create a new app with the provided configuration.
func NewAppWithConfig(config AppConfig) *App {
	router := config.Router
	if router == nil {
		router = mux.NewRouter()
	}

	interval := config.Interval
	if interval == 0 {
		interval = defaultRegisterInterval
	}

	registration := &registry.AppRegistration{Name: config.Name, Host: config.ListenIP, Port: config.ListenPort}
	heartbeater := registry.NewHeartbeater(registration, config.Registry, interval)

	return &App{
		Config:      config,
		router:      router,
		heartbeater: heartbeater,
		stats:       newAppStats(config.Client),
	}
}

// Register a handler function.
//
// If vulcan registration is enabled in the both app config and handler spec,
// the handler will be registered in the local etcd instance.
func (app *App) AddHandler(spec Spec) error {
	var handler http.HandlerFunc

	// make a handler depending on the function provided in the spec
	if spec.RawHandler != nil {
		handler = spec.RawHandler
	} else if spec.Handler != nil {
		handler = MakeHandler(app, spec.Handler, spec)
	} else if spec.HandlerWithBody != nil {
		handler = MakeHandlerWithBody(app, spec.HandlerWithBody, spec)
	} else {
		return fmt.Errorf("the spec does not provide a handler function: %v", spec)
	}

	for _, path := range spec.Paths {
		// register a handler in the router
		route := app.router.HandleFunc(path, handler).Methods(spec.Methods...)
		if len(spec.Headers) != 0 {
			route.Headers(spec.Headers...)
		}

		app.registerLocation(spec.Methods, path, spec.Scopes, spec.Middlewares)
	}

	return nil
}

// GetHandler returns HTTP compatible Handler interface.
func (app *App) GetHandler() http.Handler {
	return app.router
}

// SetNotFoundHandler sets the handler for the case when URL can not be matched by the router.
func (app *App) SetNotFoundHandler(fn http.HandlerFunc) {
	app.router.NotFoundHandler = fn
}

// IsPublicRequest determines whether the provided request came through the public HTTP endpoint.
func (app *App) IsPublicRequest(request *http.Request) bool {
	return request.Host == app.Config.PublicAPIHost
}

// Start the app on the configured host/port.
//
// Supports graceful shutdown on 'kill' and 'int' signals.
func (app *App) Run() error {
	// toggle heartbeat on SIGUSR1
	go func() {
		app.heartbeater.Start()
		heartbeatChan := make(chan os.Signal, 1)
		signal.Notify(heartbeatChan, syscall.SIGUSR1)

		for s := range heartbeatChan {
			log.Infof("Received signal: %v, toggling heartbeat", s)
			app.heartbeater.Toggle()
		}
	}()

	// listen for a shutdown signal
	go func() {
		exitChan := make(chan os.Signal, 1)
		signal.Notify(exitChan, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		s := <-exitChan
		log.Infof("Got shutdown signal: %v", s)
		manners.Close()
	}()

	addr := fmt.Sprintf("%v:%v", app.Config.ListenIP, app.Config.ListenPort)
	return manners.ListenAndServe(addr, app.router)
}

// registerLocation is a helper for registering handlers in vulcan.
func (app *App) registerLocation(methods []string, path string, scopes []Scope, middlewares []middleware.Middleware) {
	for _, scope := range scopes {
		app.registerLocationForScope(methods, path, scope, middlewares)
	}
}

// registerLocationForScope registers a location with a specified scope.
func (app *App) registerLocationForScope(methods []string, path string, scope Scope, middlewares []middleware.Middleware) {
	host, err := app.apiHostForScope(scope)
	if err != nil {
		log.Errorf("Failed to register a location: %v", err)
		return
	}
	app.registerLocationForHost(methods, path, host, middlewares)
}

// registerLocationForHost registers a location for a specified hostname.
func (app *App) registerLocationForHost(methods []string, path, host string, middlewares []middleware.Middleware) {
	r := &registry.HandlerRegistration{
		Name:        app.Config.Name,
		Host:        host,
		Path:        path,
		Methods:     methods,
		Middlewares: middlewares,
	}
	app.Config.Registry.RegisterHandler(r)

	log.Infof("Registered: %v", r)
}

// apiHostForScope is a helper that returns an appropriate API hostname for a provided scope.
func (app *App) apiHostForScope(scope Scope) (string, error) {
	if scope == ScopePublic {
		return app.Config.PublicAPIHost, nil
	} else if scope == ScopeProtected {
		return app.Config.ProtectedAPIHost, nil
	} else {
		return "", fmt.Errorf("unknown scope value: %v", scope)
	}
}
