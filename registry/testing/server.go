// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testing provides a fake implementation of the registry API.

package testing

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	ErrMissingToken   = errors.New("missing token")
	ErrBadToken       = errors.New("bad token")
	ErrBadCredentials = errors.New("bad credentials")
)

type Repository struct {
	Name     string
	Tags     map[string]string
	Username string
	Password string
	Token    string
	Expire   int
	// OCI makes the tags OCI manifests: as distribution does, the registry
	// then answers 404 to a manifest request whose Accept header does not
	// list an OCI media type.
	OCI bool
	// TagsPageSize, when positive, makes the tag listing answer that many
	// tags at a time, with a Link header to the next page.
	TagsPageSize int
}

type tagListResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type RegistryServer struct {
	listener      net.Listener
	muxer         *mux.Router
	Repos         []Repository
	reposLock     sync.RWMutex
	storageDelete bool
	tokenAuth     bool
	tokenRenew    bool
}

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	IssuedAt  string `json:"issued_at"`
}

// NewServer returns a new instance of the fake server.
//
// It receives the bind address (use 127.0.0.1:0 for getting an available port
// on the host)
func NewServer(bind string) (*RegistryServer, error) {
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	server := RegistryServer{
		listener:      listener,
		storageDelete: true,
	}
	server.buildMuxer()
	go http.Serve(listener, &server)
	return &server, nil
}

// Stop stops the server, cleaning the internal listener and freeing the
// allocated port.
func (s *RegistryServer) Stop() error {
	return s.listener.Close()
}

// Reset resets all internal information of the server.
func (s *RegistryServer) Reset() {
	s.reposLock.Lock()
	s.Repos = nil
	s.storageDelete = true
	s.reposLock.Unlock()
}

// Addr returns the Address of the server.
func (s *RegistryServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *RegistryServer) AddRepo(r Repository) {
	s.Repos = append(s.Repos, r)
}

func (s *RegistryServer) SetStorageDelete(sd bool) {
	s.reposLock.Lock()
	s.storageDelete = sd
	s.reposLock.Unlock()
}

func (s *RegistryServer) SetTokenAuth(enabled bool, renew bool) {
	s.tokenAuth = enabled
	s.tokenRenew = renew
}

// ServeHTTP handler HTTP requests, dealing with prepared failures before
// dispatching the request to the proper internal handler.
func (s *RegistryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.muxer.ServeHTTP(w, r)
}

func (s *RegistryServer) buildMuxer() {
	s.muxer = mux.NewRouter()
	s.muxer.Path("/v2/{name:.*}/manifests/{tag:.*}").Methods("HEAD").HandlerFunc(s.getDigest)
	s.muxer.Path("/v2/{name:.*}/manifests/{digest:.*}").Methods("DELETE").HandlerFunc(s.removeTag)
	s.muxer.Path("/v2/{name:.*}/tags/list").Methods("GET").HandlerFunc(s.listTags)
	s.muxer.Path("/token/{name:.*}").Methods("GET").HandlerFunc(s.getToken)
}

func (s *RegistryServer) auth(r *http.Request) error {
	name := mux.Vars(r)["name"]
	repo, _ := s.findRepository(name)
	if s.tokenAuth {
		authTokenHeader := r.Header.Get("Authorization")
		if authTokenHeader == "" || strings.Contains(authTokenHeader, "Basic") {
			return ErrMissingToken
		}
		if authTokenHeader != "Bearer "+repo.Token {
			return ErrBadToken
		}
		if authTokenHeader == "Bearer "+repo.Token {
			return nil
		}
	}

	if len(repo.Username) == 0 && len(repo.Password) == 0 {
		return nil
	}

	authHeader := r.Header.Get("Authorization")
	credentials := fmt.Sprintf("%s:%s", repo.Username, repo.Password)
	b64Credentials := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
	if authHeader != b64Credentials {
		return ErrBadCredentials
	}
	return nil
}

func (s *RegistryServer) getToken(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	repo, _ := s.findRepository(name)
	scope := r.URL.Query().Get("scope")
	service := r.URL.Query().Get("service")

	if scope == "" || service == "" {
		http.Error(w, "missing scope or service", http.StatusBadRequest)
		return
	}

	issued_at := time.Now().Format(time.RFC3339)
	if s.tokenRenew {
		issued_at = "2017-08-29T00:00:00Z"
		s.tokenRenew = false
	}

	response := TokenResponse{Token: repo.Token, ExpiresIn: repo.Expire, IssuedAt: issued_at}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *RegistryServer) handleAuthError(w http.ResponseWriter, err error, name string) {
	cause := errors.Cause(err)
	if cause == ErrMissingToken {
		w.Header().Set("Www-Authenticate", "Bearer realm=\"http://"+s.Addr()+"/token/"+name+"\",service=\""+s.Addr()+"\",scope=\"repository:"+name+":push\"")
		w.WriteHeader(http.StatusUnauthorized)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}
}
func (s *RegistryServer) removeTag(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	repo, index := s.findRepository(name)
	err := s.auth(r)
	if err != nil {
		s.handleAuthError(w, err, name)
		return
	}

	digest := mux.Vars(r)["digest"]
	if index < 0 {
		http.Error(w, fmt.Sprintf("unknown repository name=%s", name), http.StatusNotFound)
		return
	}
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	if !s.storageDelete {
		http.Error(w, "storage delete is disabled", http.StatusMethodNotAllowed)
		return
	}
	// A manifest goes with every tag that points at it, as in distribution.
	found := false
	for t, d := range repo.Tags {
		if digest == d {
			delete(repo.Tags, t)
			found = true
		}
	}
	if found {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	http.Error(w, fmt.Sprintf("unknown manifest=%s", digest), http.StatusNotFound)
}

func (s *RegistryServer) getDigest(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	repo, index := s.findRepository(name)
	err := s.auth(r)
	if err != nil {
		s.handleAuthError(w, err, name)
		return
	}
	tag := mux.Vars(r)["tag"]
	if index < 0 {
		http.Error(w, fmt.Sprintf("unknown repository name=%s", name), http.StatusNotFound)
		return
	}
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	if repo.OCI && !strings.Contains(strings.Join(r.Header.Values("Accept"), ","), "application/vnd.oci.image.manifest.v1+json") {
		http.Error(w, "OCI manifest found, but accept header does not support OCI manifests", http.StatusNotFound)
		return
	}
	for t, digest := range repo.Tags {
		if t == tag {
			w.Header().Set("Docker-Content-Digest", digest)
			return
		}
	}
	http.Error(w, fmt.Sprintf("unknown tag=%s", tag), http.StatusNotFound)
}

func (s *RegistryServer) listTags(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	repo, index := s.findRepository(name)
	err := s.auth(r)
	if err != nil {
		s.handleAuthError(w, err, name)
		return
	}
	if index < 0 {
		http.Error(w, fmt.Sprintf("unknown repository name=%s", name), http.StatusNotFound)
		return
	}
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	tags := make([]string, 0, len(repo.Tags))
	for t := range repo.Tags {
		tags = append(tags, t)
	}
	if repo.TagsPageSize > 0 {
		sort.Strings(tags)
		if last := r.URL.Query().Get("last"); last != "" {
			tags = tags[sort.SearchStrings(tags, last+"\x00"):]
		}
		if len(tags) > repo.TagsPageSize {
			tags = tags[:repo.TagsPageSize]
			w.Header().Set("Link", fmt.Sprintf(`</v2/%s/tags/list?last=%s>; rel="next"`, repo.Name, tags[len(tags)-1]))
		}
	}
	err = json.NewEncoder(w).Encode(tagListResponse{Name: repo.Name, Tags: tags})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *RegistryServer) findRepository(name string) (Repository, int) {
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	for i, repo := range s.Repos {
		if repo.Name == name {
			return repo, i
		}
	}
	return Repository{}, -1
}
