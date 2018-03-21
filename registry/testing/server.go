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
	"sync"

	"github.com/gorilla/mux"
)

type Repository struct {
	Name     string
	Tags     map[string]string
	Username string
	Password string
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
}

func (s *RegistryServer) auth(w http.ResponseWriter, r *http.Request) error {
	name := mux.Vars(r)["name"]
	repo, _ := s.findRepository(name)
	if len(repo.Username) == 0 && len(repo.Password) == 0 {
		return nil
	}

	authHeader := r.Header.Get("Authorization")
	credentials := fmt.Sprintf("%s:%s", repo.Username, repo.Password)
	b64Credentials := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
	if authHeader != b64Credentials {
		return fmt.Errorf("bad credentials")
	}
	return nil
}

func (s *RegistryServer) removeTag(w http.ResponseWriter, r *http.Request) {
	err := s.auth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	name := mux.Vars(r)["name"]
	digest := mux.Vars(r)["digest"]
	repo, index := s.findRepository(name)
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
	for t, d := range repo.Tags {
		if digest == d {
			delete(repo.Tags, t)
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}
	http.Error(w, fmt.Sprintf("unknown manifest=%s", digest), http.StatusNotFound)
}

func (s *RegistryServer) getDigest(w http.ResponseWriter, r *http.Request) {
	err := s.auth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	name := mux.Vars(r)["name"]
	tag := mux.Vars(r)["tag"]
	repo, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, fmt.Sprintf("unknown repository name=%s", name), http.StatusNotFound)
		return
	}
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	for t, digest := range repo.Tags {
		if t == tag {
			w.Header().Set("Docker-Content-Digest", digest)
			return
		}
	}
	http.Error(w, fmt.Sprintf("unknown tag=%s", tag), http.StatusNotFound)
}

func (s *RegistryServer) listTags(w http.ResponseWriter, r *http.Request) {
	err := s.auth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	name := mux.Vars(r)["name"]
	repo, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, fmt.Sprintf("unknown repository name=%s", name), http.StatusNotFound)
		return
	}
	s.reposLock.RLock()
	defer s.reposLock.RUnlock()
	tags := make([]string, len(repo.Tags))
	i := 0
	for t := range repo.Tags {
		tags[i] = t
		i++
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
