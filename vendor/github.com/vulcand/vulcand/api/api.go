package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/vulcand/vulcand/anomaly"
	"github.com/vulcand/vulcand/engine"
	"github.com/vulcand/vulcand/plugin"
	"github.com/vulcand/vulcand/router"
)

type ProxyController struct {
	ng    engine.Engine
	stats engine.StatsProvider
}

func InitProxyController(ng engine.Engine, stats engine.StatsProvider, router *mux.Router) {
	c := &ProxyController{ng: ng, stats: stats}

	router.NotFoundHandler = http.HandlerFunc(c.handleError)

	router.HandleFunc("/v1/status", handlerWithBody(c.getStatus)).Methods("GET")
	router.HandleFunc("/v2/status", handlerWithBody(c.getStatus)).Methods("GET")

	router.HandleFunc("/v2/pprof/heap", http.HandlerFunc(getHeapProfile)).Methods("GET")

	router.HandleFunc("/v2/log/severity", handlerWithBody(c.getLogSeverity)).Methods("GET")
	router.HandleFunc("/v2/log/severity", handlerWithBody(c.updateLogSeverity)).Methods("PUT")

	// Hosts
	router.HandleFunc("/v2/hosts", handlerWithBody(c.upsertHost)).Methods("POST")
	router.HandleFunc("/v2/hosts", handlerWithBody(c.getHosts)).Methods("GET")
	router.HandleFunc("/v2/hosts/{hostname}", handlerWithBody(c.getHost)).Methods("GET")
	router.HandleFunc("/v2/hosts/{hostname}", handlerWithBody(c.deleteHost)).Methods("DELETE")

	// Listeners
	router.HandleFunc("/v2/listeners", handlerWithBody(c.getListeners)).Methods("GET")
	router.HandleFunc("/v2/listeners", handlerWithBody(c.upsertListener)).Methods("POST")
	router.HandleFunc("/v2/listeners/{id}", handlerWithBody(c.getListener)).Methods("GET")
	router.HandleFunc("/v2/listeners/{id}", handlerWithBody(c.deleteListener)).Methods("DELETE")

	// Top provides top-style realtime statistics about frontends and servers
	router.HandleFunc("/v2/top/frontends", handlerWithBody(c.getTopFrontends)).Methods("GET")
	router.HandleFunc("/v2/top/servers", handlerWithBody(c.getTopServers)).Methods("GET")

	// Frontends
	router.HandleFunc("/v2/frontends", handlerWithBody(c.upsertFrontend)).Methods("POST")
	router.HandleFunc("/v2/frontends/{id}", handlerWithBody(c.getFrontend)).Methods("GET")
	router.HandleFunc("/v2/frontends", handlerWithBody(c.getFrontends)).Methods("GET")
	router.HandleFunc("/v2/frontends/{id}", handlerWithBody(c.deleteFrontend)).Methods("DELETE")

	// Backends
	router.HandleFunc("/v2/backends", handlerWithBody(c.upsertBackend)).Methods("POST")
	router.HandleFunc("/v2/backends", handlerWithBody(c.getBackends)).Methods("GET")
	router.HandleFunc("/v2/backends/{id}", handlerWithBody(c.deleteBackend)).Methods("DELETE")
	router.HandleFunc("/v2/backends/{id}", handlerWithBody(c.getBackend)).Methods("GET")

	// Servers
	router.HandleFunc("/v2/backends/{backendId}/servers", handlerWithBody(c.getServers)).Methods("GET")
	router.HandleFunc("/v2/backends/{backendId}/servers", handlerWithBody(c.upsertServer)).Methods("POST")
	router.HandleFunc("/v2/backends/{backendId}/servers/{id}", handlerWithBody(c.getServer)).Methods("GET")
	router.HandleFunc("/v2/backends/{backendId}/servers/{id}", handlerWithBody(c.deleteServer)).Methods("DELETE")

	// Middlewares
	router.HandleFunc("/v2/frontends/{frontend}/middlewares", handlerWithBody(c.upsertMiddleware)).Methods("POST")
	router.HandleFunc("/v2/frontends/{frontend}/middlewares/{id}", handlerWithBody(c.getMiddleware)).Methods("GET")
	router.HandleFunc("/v2/frontends/{frontend}/middlewares", handlerWithBody(c.getMiddlewares)).Methods("GET")
	router.HandleFunc("/v2/frontends/{frontend}/middlewares/{id}", handlerWithBody(c.deleteMiddleware)).Methods("DELETE")
}

func (c *ProxyController) handleError(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, Response{"message": "Object not found"}, http.StatusNotFound)
}

func (c *ProxyController) getStatus(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	return Response{
		"Status": "ok",
	}, nil
}

func (c *ProxyController) getLogSeverity(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	return Response{
		"severity": c.ng.GetLogSeverity().String(),
	}, nil
}

func (c *ProxyController) updateLogSeverity(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	sev, err := log.ParseLevel(strings.ToLower(r.Form.Get("severity")))
	if err != nil {
		return nil, err
	}
	c.ng.SetLogSeverity(sev)
	return Response{"message": fmt.Sprintf("Severity has been updated to %v", sev.String())}, nil
}

func (c *ProxyController) getHosts(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	hosts, err := c.ng.GetHosts()
	return Response{
		"Hosts": hosts,
	}, err
}

func (c *ProxyController) getHost(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	h, err := c.ng.GetHost(engine.HostKey{Name: params["hostname"]})
	if err != nil {
		return nil, err
	}
	return formatResult(h, err)
}

func (c *ProxyController) getFrontends(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	fs, err := c.ng.GetFrontends()
	if err != nil {
		return nil, err
	}
	return Response{
		"Frontends": fs,
	}, nil
}

func (c *ProxyController) getTopFrontends(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	limit, err := strconv.Atoi(formGet(r.Form, "limit", "0"))
	if err != nil {
		return nil, err
	}
	var bk *engine.BackendKey
	if key := r.Form.Get("backendId"); key != "" {
		bk = &engine.BackendKey{Id: key}
	}
	frontends, err := c.stats.TopFrontends(bk)
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(frontends) {
		frontends = frontends[:limit]
	}
	return Response{
		"Frontends": frontends,
	}, nil
}

func (c *ProxyController) getFrontend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	return formatResult(c.ng.GetFrontend(engine.FrontendKey{Id: params["id"]}))
}

func (c *ProxyController) upsertHost(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	host, err := parseHostPack(body)
	if err != nil {
		return nil, err
	}
	log.Infof("Upsert %s", host)
	return formatResult(host, c.ng.UpsertHost(*host))
}

func (c *ProxyController) getListeners(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	ls, err := c.ng.GetListeners()
	return Response{
		"Listeners": ls,
	}, err
}

func (c *ProxyController) upsertListener(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	listener, err := parseListenerPack(body)
	if err != nil {
		return nil, err
	}
	log.Infof("Upsert %s", listener)
	return formatResult(listener, c.ng.UpsertListener(*listener))
}

func (c *ProxyController) getListener(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	log.Infof("Get Listener(id=%s)", params["id"])
	return formatResult(c.ng.GetListener(engine.ListenerKey{Id: params["id"]}))
}

func (c *ProxyController) deleteListener(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	log.Infof("Delete Listener(id=%s)", params["id"])
	if err := c.ng.DeleteListener(engine.ListenerKey{Id: params["id"]}); err != nil {
		return nil, err
	}
	return Response{"message": "Listener deleted"}, nil
}

func (c *ProxyController) deleteHost(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	hostname := params["hostname"]
	log.Infof("Delete host: %s", hostname)
	if err := c.ng.DeleteHost(engine.HostKey{Name: hostname}); err != nil {
		return nil, err
	}
	return Response{"message": fmt.Sprintf("Host '%s' deleted", hostname)}, nil
}

func (c *ProxyController) upsertBackend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	b, err := parseBackendPack(body)
	if err != nil {
		return nil, err
	}
	log.Infof("Upsert Backend: %s", b)
	return formatResult(b, c.ng.UpsertBackend(*b))
}

func (c *ProxyController) deleteBackend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	backendId := params["id"]
	log.Infof("Delete Backend(id=%s)", backendId)
	if err := c.ng.DeleteBackend(engine.BackendKey{Id: backendId}); err != nil {
		return nil, err
	}
	return Response{"message": "Backend deleted"}, nil
}

func (c *ProxyController) getBackends(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	backends, err := c.ng.GetBackends()
	return Response{
		"Backends": backends,
	}, err
}

func (c *ProxyController) getTopServers(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	limit, err := strconv.Atoi(formGet(r.Form, "limit", "0"))
	if err != nil {
		return nil, err
	}
	var bk *engine.BackendKey
	if key := r.Form.Get("backendId"); key != "" {
		bk = &engine.BackendKey{Id: key}
	}
	servers, err := c.stats.TopServers(bk)
	if err != nil {
		return nil, err
	}
	if bk != nil {
		anomaly.MarkServerAnomalies(servers)
	}
	if limit > 0 && limit < len(servers) {
		servers = servers[:limit]
	}
	return Response{
		"Servers": servers,
	}, nil
}

func (c *ProxyController) getBackend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	return formatResult(c.ng.GetBackend(engine.BackendKey{Id: params["id"]}))
}

func (c *ProxyController) upsertFrontend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	frontend, ttl, err := parseFrontendPack(c.ng.GetRegistry().GetRouter(), body)
	if err != nil {
		return nil, err
	}
	log.Infof("Upsert %s", frontend)
	return formatResult(frontend, c.ng.UpsertFrontend(*frontend, ttl))
}

func (c *ProxyController) deleteFrontend(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	log.Infof("Delete Frontend(id=%s)", params["id"])
	if err := c.ng.DeleteFrontend(engine.FrontendKey{Id: params["id"]}); err != nil {
		return nil, err
	}
	return Response{"message": "Frontend deleted"}, nil
}

func (c *ProxyController) upsertServer(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	backendId := params["backendId"]
	srv, ttl, err := parseServerPack(body)
	if err != nil {
		return nil, err
	}
	bk := engine.BackendKey{Id: backendId}
	log.Infof("Upsert %v %v", bk, srv)
	return formatResult(srv, c.ng.UpsertServer(bk, *srv, ttl))
}

func (c *ProxyController) getServer(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	sk := engine.ServerKey{BackendKey: engine.BackendKey{Id: params["backendId"]}, Id: params["id"]}
	log.Infof("getServer %v", sk)
	srv, err := c.ng.GetServer(sk)
	if err != nil {
		return nil, err
	}
	return formatResult(srv, err)
}

func (c *ProxyController) getServers(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	srvs, err := c.ng.GetServers(engine.BackendKey{Id: params["backendId"]})
	if err != nil {
		return nil, err
	}
	return Response{
		"Servers": srvs,
	}, nil
}

func (c *ProxyController) deleteServer(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	sk := engine.ServerKey{BackendKey: engine.BackendKey{Id: params["backendId"]}, Id: params["id"]}
	log.Infof("Delete %v", sk)
	if err := c.ng.DeleteServer(sk); err != nil {
		return nil, err
	}
	return Response{"message": "Server deleted"}, nil
}

func (c *ProxyController) upsertMiddleware(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	frontend := params["frontend"]
	m, ttl, err := parseMiddlewarePack(body, c.ng.GetRegistry())
	if err != nil {
		return nil, err
	}
	return formatResult(m, c.ng.UpsertMiddleware(engine.FrontendKey{Id: frontend}, *m, ttl))
}

func (c *ProxyController) getMiddleware(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	fk := engine.MiddlewareKey{Id: params["id"], FrontendKey: engine.FrontendKey{Id: params["frontend"]}}
	return formatResult(c.ng.GetMiddleware(fk))
}

func (c *ProxyController) getMiddlewares(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	fk := engine.FrontendKey{Id: params["frontend"]}
	out, err := c.ng.GetMiddlewares(fk)
	if err != nil {
		return nil, err
	}
	return Response{
		"Middlewares": out,
	}, nil
}

func (c *ProxyController) deleteMiddleware(w http.ResponseWriter, r *http.Request, params map[string]string, body []byte) (interface{}, error) {
	fk := engine.MiddlewareKey{Id: params["id"], FrontendKey: engine.FrontendKey{Id: params["frontend"]}}
	if err := c.ng.DeleteMiddleware(fk); err != nil {
		return nil, err
	}
	return Response{"message": "Middleware deleted"}, nil
}

func formGet(form url.Values, key, def string) string {
	if value := form.Get(key); value != "" {
		return value
	}
	return def
}

func formatResult(in interface{}, err error) (interface{}, error) {
	if err != nil {
		return nil, err
	}
	return in, nil
}

type backendPack struct {
	Backend engine.Backend
}

type backendReadPack struct {
	Backend json.RawMessage
}

type hostPack struct {
	Host engine.Host
}

type hostReadPack struct {
	Host json.RawMessage
}

type listenerPack struct {
	Listener engine.Listener
	TTL      string
}

type listenerReadPack struct {
	Listener json.RawMessage
}

type frontendReadPack struct {
	Frontend json.RawMessage
	TTL      string
}

type frontendPack struct {
	Frontend engine.Frontend
	TTL      string
}

type middlewareReadPack struct {
	Middleware json.RawMessage
	TTL        string
}

type middlewarePack struct {
	Middleware engine.Middleware
	TTL        string
}

type serverReadPack struct {
	Server json.RawMessage
	TTL    string
}

type serverPack struct {
	Server engine.Server
	TTL    string
}

func parseListenerPack(v []byte) (*engine.Listener, error) {
	var lp listenerReadPack
	if err := json.Unmarshal(v, &lp); err != nil {
		return nil, err
	}
	if len(lp.Listener) == 0 {
		return nil, &errMissingField{Field: "Listener"}
	}
	return engine.ListenerFromJSON(lp.Listener)
}

func parseHostPack(v []byte) (*engine.Host, error) {
	var hp hostReadPack
	if err := json.Unmarshal(v, &hp); err != nil {
		return nil, err
	}
	if len(hp.Host) == 0 {
		return nil, &errMissingField{Field: "Host"}
	}
	return engine.HostFromJSON(hp.Host)
}

func parseBackendPack(v []byte) (*engine.Backend, error) {
	var bp *backendReadPack
	if err := json.Unmarshal(v, &bp); err != nil {
		return nil, err
	}
	if bp == nil || len(bp.Backend) == 0 {
		return nil, &errMissingField{Field: "Backend"}
	}
	return engine.BackendFromJSON(bp.Backend)
}

func parseFrontendPack(router router.Router, v []byte) (*engine.Frontend, time.Duration, error) {
	var fp frontendReadPack
	if err := json.Unmarshal(v, &fp); err != nil {
		return nil, 0, err
	}
	if len(fp.Frontend) == 0 {
		return nil, 0, &errMissingField{Field: "Frontend"}
	}
	f, err := engine.FrontendFromJSON(router, fp.Frontend)
	if err != nil {
		return nil, 0, err
	}

	var ttl time.Duration
	if fp.TTL != "" {
		ttl, err = time.ParseDuration(fp.TTL)
		if err != nil {
			return nil, 0, err
		}
	}
	return f, ttl, nil
}

func parseMiddlewarePack(v []byte, r *plugin.Registry) (*engine.Middleware, time.Duration, error) {
	var mp middlewareReadPack
	if err := json.Unmarshal(v, &mp); err != nil {
		return nil, 0, err
	}
	if len(mp.Middleware) == 0 {
		return nil, 0, &errMissingField{Field: "Middleware"}
	}
	f, err := engine.MiddlewareFromJSON(mp.Middleware, r.GetSpec)
	if err != nil {
		return nil, 0, err
	}
	var ttl time.Duration
	if mp.TTL != "" {
		ttl, err = time.ParseDuration(mp.TTL)
		if err != nil {
			return nil, 0, err
		}
	}
	return f, ttl, nil
}

func parseServerPack(v []byte) (*engine.Server, time.Duration, error) {
	var sp serverReadPack
	if err := json.Unmarshal(v, &sp); err != nil {
		return nil, 0, err
	}
	if len(sp.Server) == 0 {
		return nil, 0, &errMissingField{Field: "Server"}
	}
	s, err := engine.ServerFromJSON(sp.Server)
	if err != nil {
		return nil, 0, err
	}
	var ttl time.Duration
	if sp.TTL != "" {
		ttl, err = time.ParseDuration(sp.TTL)
		if err != nil {
			return nil, 0, err
		}
	}
	return s, ttl, nil
}

// getHeapProfile responds with a pprof-formatted heap profile.
func getHeapProfile(w http.ResponseWriter, r *http.Request) {
	// Ensure up-to-date data.
	runtime.GC()
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := pprof.Lookup("heap").WriteTo(w, 0); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not get heap profile: %s\n", err)
	}
}

type handlerWithBodyFn func(http.ResponseWriter, *http.Request, map[string]string, []byte) (interface{}, error)

func handlerWithBody(fn handlerWithBodyFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := parseForm(r); err != nil {
			sendResponse(w, fmt.Sprintf("failed to parse request, err=%v", err), http.StatusInternalServerError)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			sendResponse(w, fmt.Sprintf("failed to read request body, err=%v", err), http.StatusInternalServerError)
			return
		}

		rs, err := fn(w, r, mux.Vars(r), body)
		if err != nil {
			var status int
			switch err.(type) {
			case *engine.InvalidFormatError:
				status = http.StatusBadRequest
			case errMissingField:
				status = http.StatusBadRequest
			case *engine.NotFoundError:
				status = http.StatusNotFound
			case *engine.AlreadyExistsError:
				status = http.StatusConflict
			default:
				status = http.StatusInternalServerError
			}
			sendResponse(w, Response{"message": err.Error()}, status)
			return
		}
		sendResponse(w, rs, http.StatusOK)
	}
}

type Response map[string]interface{}

// Reply with the provided HTTP response and status code.
//
// Response body must be JSON-marshallable, otherwise the response
// will be "Internal Server Error".
func sendResponse(w http.ResponseWriter, response interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	marshalledResponse, err := json.Marshal(response)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Failed to marshal response: %v %v", response, err)))
		return
	}
	w.WriteHeader(status)
	w.Write(marshalledResponse)
}

// parseForm the request data based on its content type.
func parseForm(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") == true {
		return r.ParseMultipartForm(0)
	}
	return r.ParseForm()
}

type errMissingField struct {
	Field string
}

func (e errMissingField) Error() string {
	return fmt.Sprintf("Missing mandatory parameter: %v", e.Field)
}
