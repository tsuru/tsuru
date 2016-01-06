/*
Package manners provides a wrapper for a standard net/http server that
ensures all active HTTP client have completed their current request
before the server shuts down.

It can be used a drop-in replacement for the standard http package,
or can wrap a pre-configured Server.

eg.
	myHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	  w.Write([]byte("Hello\n"))
	})

	http.Handle("/hello", myHandler)

	log.Fatal(manners.ListenAndServe(":8080", nil))

or for a customized server:
	s := manners.NewWithServer(&http.Server{
		Addr:           ":8080",
		Handler:        myHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	})
	log.Fatal(s.ListenAndServe())


The server will shutdown cleanly when the Close() method is called:

	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt, os.Kill)
		<-sigchan
		log.Info("Shutting down...")
		manners.Close()
	}()

	http.Handle("/hello", myHandler)
	log.Fatal(manners.ListenAndServe(":8080", nil))
*/
package manners

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
)

// interface describing a waitgroup, so unit
// tests can mock out an instrumentable version
type waitgroup interface {
	Add(delta int)
	Done()
	Wait()
}

// StateHandler can be called by the server if the state of the connection changes.
// Notice that it passed previous state and the new state as parameters.
type StateHandler func(net.Conn, http.ConnState, http.ConnState)

type Options struct {
	Server       *http.Server
	StateHandler StateHandler
	Listener     net.Listener
}

// NewServer creates a new GracefulServer. The server will begin shutting down when
// a value is passed to the Shutdown channel.
func NewServer() *GracefulServer {
	return NewWithServer(new(http.Server))
}

// NewWithServer wraps an existing http.Server object and returns a GracefulServer
// that supports all of the original Server operations.
func NewWithServer(s *http.Server) *GracefulServer {
	return &GracefulServer{
		Server:   s,
		shutdown: make(chan struct{}),
		wg:       new(sync.WaitGroup),
	}
}

func NewWithOptions(o Options) *GracefulServer {
	// Set up listener
	var listener *GracefulListener
	if o.Listener != nil {
		g, ok := o.Listener.(*GracefulListener)
		if !ok {
			listener = NewListener(o.Listener)
		} else {
			listener = g
		}
	}

	return &GracefulServer{
		listener:     listener,
		Server:       o.Server,
		stateHandler: o.StateHandler,
		shutdown:     make(chan struct{}),
		wg:           new(sync.WaitGroup),
	}
}

// A GracefulServer maintains a WaitGroup that counts how many in-flight
// requests the server is handling. When it receives a shutdown signal,
// it stops accepting new requests but does not actually shut down until
// all in-flight requests terminate.
//
// GracefulServer embeds the underlying net/http.Server making its non-override
// methods and properties avaiable.
//
// It must be initialized by calling NewServer or NewWithServer
type GracefulServer struct {
	*http.Server
	shutdown chan struct{}
	wg       waitgroup
	listener *GracefulListener

	// used by test code
	up chan net.Listener

	stateHandler StateHandler
}

// Close stops the server from accepting new requets and beings shutting down.
func (s *GracefulServer) Close() {
	close(s.shutdown)
}

// ListenAndServe provides a graceful equivalent of net/http.Serve.ListenAndServe.
func (s *GracefulServer) ListenAndServe() error {
	if s.listener == nil {
		oldListener, err := net.Listen("tcp", s.Addr)
		if err != nil {
			return err
		}
		s.listener = NewListener(oldListener.(*net.TCPListener))
	}
	return s.Serve(s.listener)
}

// ListenAndServeTLS provides a graceful equivalent of net/http.Serve.ListenAndServeTLS.
func (s *GracefulServer) ListenAndServeTLS(certFile, keyFile string) error {
	addr := s.Addr
	if addr == "" {
		addr = ":https"
	}
	config := &tls.Config{}
	if s.TLSConfig != nil {
		*config = *s.TLSConfig
	}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	return s.ListenAndServeTLSWithConfig(config)
}

// ListenAndServeTLS provides a graceful equivalent of net/http.Serve.ListenAndServeTLS.
func (s *GracefulServer) ListenAndServeTLSWithConfig(config *tls.Config) error {
	addr := s.Addr
	if addr == "" {
		addr = ":https"
	}

	if s.listener == nil {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		tlsListener := NewTLSListener(TCPKeepAliveListener{ln.(*net.TCPListener)}, config)
		s.listener = NewListener(tlsListener)
	}
	return s.Serve(s.listener)
}

func (gs *GracefulServer) GetFile() (*os.File, error) {
	return gs.listener.GetFile()
}

func (gs *GracefulServer) HijackListener(s *http.Server, config *tls.Config) (*GracefulServer, error) {
	listener, err := gs.listener.Clone()
	if err != nil {
		return nil, err
	}

	if config != nil {
		listener = NewTLSListener(TCPKeepAliveListener{listener.(*net.TCPListener)}, config)
	}

	other := NewWithServer(s)
	other.listener = NewListener(listener)
	return other, nil
}

// Serve provides a graceful equivalent net/http.Server.Serve.
//
// If listener is not an instance of *GracefulListener is will be wrapped
// to become one.
func (s *GracefulServer) Serve(listener net.Listener) error {
	// accept a net.Listener to preserve the interface compatibility with the standard
	// http.Server, but we except a GracefluListener
	gracefulListener, ok := listener.(*GracefulListener)
	if !ok {
		gracefulListener = NewListener(listener)
		listener = gracefulListener
	}
	s.listener = gracefulListener

	var closing int32

	go func() {
		<-s.shutdown
		atomic.StoreInt32(&closing, 1)
		s.Server.SetKeepAlivesEnabled(false)
		listener.Close()
	}()

	orgConnState := s.Server.ConnState
	s.ConnState = func(conn net.Conn, newState http.ConnState) {
		// Ugly hack, but it works. We pass the information about the underlying state via the only available interface in net.Conn
		// we do this not to override the tls.Conn, as the internal logic of http.Server depends on the type assertion (unfortunately)
		gconn := conn.LocalAddr().(*gracefulAddr).gconn
		switch newState {
		case http.StateNew:
			// new_conn -> StateNew
			s.StartRoutine()

		case http.StateActive:
			// (StateNew, StateIdle) -> StateActive
			if gconn.lastHTTPState == http.StateIdle {
				// transitioned from idle back to active
				s.StartRoutine()
			}

		case http.StateIdle:
			// StateActive -> StateIdle
			if atomic.LoadInt32(&closing) == 1 {
				// rapidly close newly idle connections; if not they may make
				// one more request before SetKeepAliveEnabled(false)  takes effect.
				conn.Close()
			}
			s.FinishRoutine()

		case http.StateClosed, http.StateHijacked:
			// (StateNew, StateActive, StateIdle) -> (StateClosed, StateHiJacked)
			if gconn.lastHTTPState != http.StateIdle {
				// if it was idle it's already been decremented
				s.FinishRoutine()
			}
		}
		if s.stateHandler != nil {
			s.stateHandler(conn, gconn.lastHTTPState, newState)
		}
		gconn.lastHTTPState = newState
		if orgConnState != nil {
			orgConnState(conn, newState)
		}
	}

	// only used by unit tests
	if s.up != nil {
		// notify test that server is up; wait for signal to continue
		s.up <- listener
	}
	err := s.Server.Serve(listener)

	// This block is reached when the server has received a shut down command.
	if err == nil {
		s.wg.Wait()
		return nil
	} else if _, ok := err.(listenerAlreadyClosed); ok {
		s.wg.Wait()
		return nil
	}
	return err
}

// StartRoutine increments the server's WaitGroup. Use this if a web request starts more
// goroutines and these goroutines are not guaranteed to finish before the
// request.
func (s *GracefulServer) StartRoutine() {
	s.wg.Add(1)
}

// FinishRoutine decrements the server's WaitGroup. Used this to complement StartRoutine().
func (s *GracefulServer) FinishRoutine() {
	s.wg.Done()
}

var (
	servers []*GracefulServer
	m       sync.Mutex
)

// ListenAndServe provides a graceful version of function provided by the net/http package.
func ListenAndServe(addr string, handler http.Handler) error {
	server := NewWithServer(&http.Server{Addr: addr, Handler: handler})
	m.Lock()
	servers = append(servers, server)
	m.Unlock()
	return server.ListenAndServe()
}

// ListenAndServeTLS provides a graceful version of function provided by the net/http package.
func ListenAndServeTLS(addr string, certFile string, keyFile string, handler http.Handler) error {
	server := NewWithServer(&http.Server{Addr: addr, Handler: handler})
	m.Lock()
	servers = append(servers, server)
	m.Unlock()
	return server.ListenAndServeTLS(certFile, keyFile)
}

// Serve provides a graceful version of function provided by the net/http package.
func Serve(l net.Listener, handler http.Handler) error {
	server := NewWithServer(&http.Server{Handler: handler})
	m.Lock()
	servers = append(servers, server)
	m.Unlock()
	return server.Serve(l)
}

// Close triggers a shutdown of all running Graceful servers.
func Close() {
	m.Lock()
	for _, s := range servers {
		s.Close()
	}
	servers = nil
	m.Unlock()
}
