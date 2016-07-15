// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gandalftest provides a fake implementation of the Gandalf API.
package gandalftest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/pat"
	"github.com/tsuru/gandalf/repository"
	"github.com/tsuru/gandalf/user"
	"github.com/tsuru/tsuru/errors"
	"golang.org/x/crypto/ssh"
)

type Repository struct {
	Name          string                `json:"name"`
	Users         []string              `json:"users"`
	ReadOnlyUsers []string              `json:"readonlyusers"`
	ReadOnlyURL   string                `json:"git_url"`
	ReadWriteURL  string                `json:"ssh_url"`
	IsPublic      bool                  `json:"ispublic"`
	Diffs         chan string           `json:"-"`
	History       repository.GitHistory `json:"-"`
}

type testUser struct {
	Name string
	Keys map[string]string
}

type key struct {
	Name string
	Body string
}

// Failure represents a prepared failure, that is used in the PrepareFailure
// method.
type Failure struct {
	Code     int
	Method   string
	Path     string
	Response string
}

// GandalfServer is a fake gandalf server. An instance of the client can be
// pointed to the address generated for this server
type GandalfServer struct {
	// Host is used for building repositories URLs.
	Host string

	listener  net.Listener
	muxer     *pat.Router
	users     []string
	keys      map[string][]key
	repos     []Repository
	usersLock sync.RWMutex
	repoLock  sync.RWMutex
	failures  chan Failure
}

// NewServer returns an instance of the test server, bound to the specified
// address. To get a random port, users can specify the :0 port.
//
// Examples:
//
//     server, err := NewServer("127.0.0.1:8080") // will bind on port 8080
//     server, err := NewServer("127.0.0.1:0") // will get a random available port
func NewServer(bind string) (*GandalfServer, error) {
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	server := GandalfServer{
		listener: listener,
		keys:     make(map[string][]key),
		failures: make(chan Failure, 1),
	}
	server.buildMuxer()
	go http.Serve(listener, &server)
	return &server, nil
}

// Stop stops the server, cleaning the internal listener and freeing the
// allocated port.
func (s *GandalfServer) Stop() error {
	return s.listener.Close()
}

// URL returns the URL of the server, in the format "http://<host>:<port>/".
func (s *GandalfServer) URL() string {
	return fmt.Sprintf("http://%s/", s.listener.Addr())
}

// PrepareFailure prepares a failure in the server. The next request matching
// the given URL and request path will fail with a 500 code and the provided
// response in the body.
func (s *GandalfServer) PrepareFailure(failure Failure) {
	s.failures <- failure
}

// ServeHTTP handler HTTP requests, dealing with prepared failures before
// dispatching the request to the proper internal handler.
func (s *GandalfServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if failure, ok := s.getFailure(r.Method, r.URL.Path); ok {
		code := failure.Code
		if code == 0 {
			code = http.StatusInternalServerError
		}
		http.Error(w, failure.Response, code)
		return
	}
	s.muxer.ServeHTTP(w, r)
}

// Users returns the list of users registered in the server.
func (s *GandalfServer) Users() []string {
	s.usersLock.RLock()
	defer s.usersLock.RUnlock()
	return s.users
}

// Repository returns the list of repositories registered in the server.
func (s *GandalfServer) Repositories() []Repository {
	s.repoLock.RLock()
	defer s.repoLock.RUnlock()
	return s.repos
}

// Grants returns a map of grant in repositories, mapping the name of the
// repository to the slice of users that have access to it.
func (s *GandalfServer) Grants() map[string][]string {
	s.repoLock.RLock()
	defer s.repoLock.RUnlock()
	result := make(map[string][]string, len(s.repos))
	for _, repo := range s.repos {
		result[repo.Name] = repo.Users
	}
	return result
}

// ReadOnlyGrants returns a map of read-only grants in repositories, mapping
// the name of the repository to the slice of users that have access to it.
func (s *GandalfServer) ReadOnlyGrants() map[string][]string {
	s.repoLock.RLock()
	defer s.repoLock.RUnlock()
	result := make(map[string][]string, len(s.repos))
	for _, repo := range s.repos {
		result[repo.Name] = repo.ReadOnlyUsers
	}
	return result
}

// Keys returns all the keys registered for the given users, or an error if the
// user doesn't exist.
func (s *GandalfServer) Keys(user string) (map[string]string, error) {
	s.usersLock.RLock()
	defer s.usersLock.RUnlock()
	keys, ok := s.keys[user]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	keyMap := make(map[string]string, len(keys))
	for _, key := range keys {
		keyMap[key.Name] = key.Body
	}
	return keyMap, nil
}

// PrepareDiff prepares a diff for the given repository and writes it to the
// next getDiff call in the server.
func (s *GandalfServer) PrepareDiff(repository, content string) {
	if repo, index := s.findRepository(repository); index > -1 {
		repo.Diffs <- content
		s.repoLock.Lock()
		s.repos[index] = repo
		s.repoLock.Unlock()
	}
}

// PrepareDiff prepares a diff for the given repository and writes it to the
// next getDiff call in the server.
func (s *GandalfServer) PrepareLogs(repository string, log repository.GitHistory) {
	if repo, index := s.findRepository(repository); index > -1 {
		s.repoLock.Lock()
		repo.History = log
		s.repos[index] = repo
		s.repoLock.Unlock()
	}
}

// Reset resets all internal information of the server, like keys, repositories, users and prepared failures.
func (s *GandalfServer) Reset() {
	s.usersLock.Lock()
	s.users = nil
	s.keys = make(map[string][]key)
	s.usersLock.Unlock()
	s.repoLock.Lock()
	s.repos = nil
	s.repoLock.Unlock()
	for {
		select {
		case <-s.failures:
		default:
			return
		}
	}
}

func (s *GandalfServer) buildMuxer() {
	s.muxer = pat.New()
	s.muxer.Put("/user/{name}/key/{keyname}", http.HandlerFunc(s.updateKey))
	s.muxer.Post("/user/{name}/key", http.HandlerFunc(s.addKeys))
	s.muxer.Delete("/user/{name}/key/{keyname}", http.HandlerFunc(s.removeKey))
	s.muxer.Get("/user/{name}/keys", http.HandlerFunc(s.listKeys))
	s.muxer.Post("/user", http.HandlerFunc(s.createUser))
	s.muxer.Delete("/user/{name}", http.HandlerFunc(s.removeUser))
	s.muxer.Post("/repository/grant", http.HandlerFunc(s.grantAccess))
	s.muxer.Delete("/repository/revoke", http.HandlerFunc(s.revokeAccess))
	s.muxer.Post("/repository", http.HandlerFunc(s.createRepository))
	s.muxer.Get("/repository/{name}/diff/commits", http.HandlerFunc(s.getDiff))
	s.muxer.Delete("/repository/{name}", http.HandlerFunc(s.removeRepository))
	s.muxer.Get("/repository/{name}/logs", http.HandlerFunc(s.getLogs))
	s.muxer.Get("/repository/{name}", http.HandlerFunc(s.getRepository))
	s.muxer.Get("/healthcheck", http.HandlerFunc(s.healthcheck))
}

func (s *GandalfServer) createUser(w http.ResponseWriter, r *http.Request) {
	s.usersLock.Lock()
	defer s.usersLock.Unlock()
	defer r.Body.Close()
	var usr testUser
	err := json.NewDecoder(r.Body).Decode(&usr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, ok := s.keys[usr.Name]; ok {
		http.Error(w, user.ErrUserAlreadyExists.Error(), http.StatusConflict)
		return
	}
	s.users = append(s.users, usr.Name)
	keys := make([]key, 0, len(usr.Keys))
	for name, body := range usr.Keys {
		keys = append(keys, key{Name: name, Body: body})
	}
	s.keys[usr.Name] = keys
}

func (s *GandalfServer) removeUser(w http.ResponseWriter, r *http.Request) {
	userName := r.URL.Query().Get(":name")
	_, index := s.findUser(userName)
	if index < 0 {
		http.Error(w, user.ErrUserNotFound.Error(), http.StatusNotFound)
		return
	}
	s.usersLock.Lock()
	defer s.usersLock.Unlock()
	last := len(s.users) - 1
	s.users[index] = s.users[last]
	s.users = s.users[:last]
	delete(s.keys, userName)
}

func (s *GandalfServer) createRepository(w http.ResponseWriter, r *http.Request) {
	var repo Repository
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo.Diffs = make(chan string, 1)
	users := append(repo.Users, repo.ReadOnlyUsers...)
	if len(users) < 1 {
		http.Error(w, "missing users", http.StatusBadRequest)
		return
	}
	for _, userName := range users {
		_, index := s.findUser(userName)
		if index < 0 {
			http.Error(w, fmt.Sprintf("user %q not found", userName), http.StatusBadRequest)
			return
		}
	}
	s.repoLock.Lock()
	defer s.repoLock.Unlock()
	for _, r := range s.repos {
		if r.Name == repo.Name {
			http.Error(w, repository.ErrRepositoryAlreadyExists.Error(), http.StatusConflict)
			return
		}
	}
	repo.ReadOnlyURL = fmt.Sprintf("git://%s/%s.git", s.Host, repo.Name)
	repo.ReadWriteURL = fmt.Sprintf("git@%s:%s.git", s.Host, repo.Name)
	s.repos = append(s.repos, repo)
}

func (s *GandalfServer) removeRepository(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get(":name")
	_, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, repository.ErrRepositoryNotFound.Error(), http.StatusNotFound)
		return
	}
	s.repoLock.Lock()
	defer s.repoLock.Unlock()
	last := len(s.repos) - 1
	s.repos[index] = s.repos[last]
	s.repos = s.repos[:last]
}

func (s *GandalfServer) getRepository(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get(":name")
	repo, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, repository.ErrRepositoryNotFound.Error(), http.StatusNotFound)
		return
	}
	err := json.NewEncoder(w).Encode(repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *GandalfServer) getLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get(":name")
	repo, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, repository.ErrRepositoryNotFound.Error(), http.StatusNotFound)
		return
	}
	s.repoLock.Lock()
	defer s.repoLock.Unlock()
	b, err := json.Marshal(repo.History)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func (s *GandalfServer) getDiff(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get(":name")
	repo, index := s.findRepository(name)
	if index < 0 {
		http.Error(w, repository.ErrRepositoryNotFound.Error(), http.StatusNotFound)
		return
	}
	var content string
	select {
	case content = <-repo.Diffs:
	default:
	}
	fmt.Fprint(w, content)
	s.repoLock.Lock()
	s.repos[index] = repo
	s.repoLock.Unlock()
}

func (s *GandalfServer) grantAccess(w http.ResponseWriter, r *http.Request) {
	readOnly := r.URL.Query().Get("readonly") == "yes"
	repositories, users, err := s.validateAccessRequest(r)
	if err != nil {
		http.Error(w, err.Message, err.Code)
		return
	}
	for _, repository := range repositories {
		repo, index := s.findRepository(repository)
		for _, user := range users {
			if s.checkUserAccess(repo, user, readOnly) < 0 {
				if readOnly {
					repo.ReadOnlyUsers = append(repo.ReadOnlyUsers, user)
				} else {
					repo.Users = append(repo.Users, user)
				}
			}
		}
		s.repoLock.Lock()
		s.repos[index] = repo
		s.repoLock.Unlock()
	}
}

func (s *GandalfServer) revokeAccess(w http.ResponseWriter, r *http.Request) {
	readOnly := r.URL.Query().Get("readonly") == "yes"
	repositories, users, err := s.validateAccessRequest(r)
	if err != nil {
		http.Error(w, err.Message, err.Code)
		return
	}
	for _, repository := range repositories {
		repo, index := s.findRepository(repository)
		for _, user := range users {
			if userAccessIndex := s.checkUserAccess(repo, user, readOnly); userAccessIndex > -1 {
				if readOnly {
					last := len(repo.ReadOnlyUsers) - 1
					repo.ReadOnlyUsers[userAccessIndex] = repo.ReadOnlyUsers[last]
					repo.ReadOnlyUsers = repo.ReadOnlyUsers[:last]
				} else {
					last := len(repo.Users) - 1
					repo.Users[userAccessIndex] = repo.Users[last]
					repo.Users = repo.Users[:last]
				}
			}
		}
		s.repoLock.Lock()
		s.repos[index] = repo
		s.repoLock.Unlock()
	}
}

func (s *GandalfServer) validateAccessRequest(r *http.Request) (repositories []string, users []string, err *errors.HTTP) {
	defer r.Body.Close()
	var params map[string][]string
	jerr := json.NewDecoder(r.Body).Decode(&params)
	if jerr != nil {
		return nil, nil, &errors.HTTP{Code: http.StatusBadRequest, Message: jerr.Error()}
	}
	users = params["users"]
	if len(users) < 1 {
		return nil, nil, &errors.HTTP{Code: http.StatusBadRequest, Message: "missing users"}
	}
	repositories = params["repositories"]
	if len(repositories) < 1 {
		return nil, nil, &errors.HTTP{Code: http.StatusBadRequest, Message: "missing repositories"}
	}
	for _, user := range users {
		_, index := s.findUser(user)
		if index < 0 {
			return nil, nil, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: fmt.Sprintf("user %q not found", user),
			}
		}
	}
	for _, repository := range repositories {
		_, index := s.findRepository(repository)
		if index < 0 {
			return nil, nil, &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: fmt.Sprintf("repository %q not found", repository),
			}
		}
	}
	return repositories, users, nil
}

func (s *GandalfServer) addKeys(w http.ResponseWriter, r *http.Request) {
	userName := r.URL.Query().Get(":name")
	var keys map[string]string
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&keys)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.usersLock.Lock()
	defer s.usersLock.Unlock()
	userKeys, ok := s.keys[userName]
	if !ok {
		http.Error(w, user.ErrUserNotFound.Error(), http.StatusNotFound)
		return
	}
	for name, body := range keys {
		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(body)); err != nil {
			http.Error(w, user.ErrInvalidKey.Error(), http.StatusBadRequest)
			return
		}
		for _, userKey := range userKeys {
			if name == userKey.Name {
				http.Error(w, user.ErrDuplicateKey.Error(), http.StatusConflict)
				return
			}
		}
	}
	for name, body := range keys {
		userKeys = append(userKeys, key{Name: name, Body: body})
	}
	s.keys[userName] = userKeys
}

func (s *GandalfServer) updateKey(w http.ResponseWriter, r *http.Request) {
	userName := r.URL.Query().Get(":name")
	keyName := r.URL.Query().Get(":keyname")
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey(body); err != nil {
		http.Error(w, user.ErrInvalidKey.Error(), http.StatusBadRequest)
		return
	}
	s.usersLock.Lock()
	defer s.usersLock.Unlock()
	userKeys, ok := s.keys[userName]
	if !ok {
		http.Error(w, user.ErrUserNotFound.Error(), http.StatusNotFound)
		return
	}
	var (
		k     key
		index int
	)
	for i, userKey := range userKeys {
		if userKey.Name == keyName {
			k = userKey
			index = i
			break
		}
	}
	if k.Name != keyName {
		http.Error(w, user.ErrKeyNotFound.Error(), http.StatusNotFound)
		return
	}
	k.Body = string(body)
	userKeys[index] = k
	s.keys[userName] = userKeys
}

func (s *GandalfServer) removeKey(w http.ResponseWriter, r *http.Request) {
	userName := r.URL.Query().Get(":name")
	keyName := r.URL.Query().Get(":keyname")
	s.usersLock.Lock()
	defer s.usersLock.Unlock()
	userKeys, ok := s.keys[userName]
	if !ok {
		http.Error(w, user.ErrUserNotFound.Error(), http.StatusNotFound)
		return
	}
	index := -1
	for i, userKey := range userKeys {
		if userKey.Name == keyName {
			index = i
			break
		}
	}
	if index < 0 {
		http.Error(w, user.ErrKeyNotFound.Error(), http.StatusNotFound)
		return
	}
	last := len(userKeys) - 1
	userKeys[index] = userKeys[last]
	s.keys[userName] = userKeys[:last]
}

func (s *GandalfServer) listKeys(w http.ResponseWriter, r *http.Request) {
	userName := r.URL.Query().Get(":name")
	s.usersLock.RLock()
	defer s.usersLock.RUnlock()
	keys, ok := s.keys[userName]
	if !ok {
		http.Error(w, user.ErrUserNotFound.Error(), http.StatusNotFound)
		return
	}
	keysMap := make(map[string]string, len(keys))
	for _, k := range keys {
		keysMap[k.Name] = k.Body
	}
	err := json.NewEncoder(w).Encode(keysMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *GandalfServer) healthcheck(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("WORKING"))
}

func (s *GandalfServer) findUser(name string) (userName string, index int) {
	s.usersLock.RLock()
	defer s.usersLock.RUnlock()
	for i, user := range s.users {
		if user == name {
			return user, i
		}
	}
	return "", -1
}

func (s *GandalfServer) checkUserAccess(repo Repository, user string, readOnly bool) int {
	list := repo.Users
	if readOnly {
		list = repo.ReadOnlyUsers
	}
	for i, userName := range list {
		if userName == user {
			return i
		}
	}
	return -1
}

func (s *GandalfServer) findRepository(name string) (Repository, int) {
	s.repoLock.RLock()
	defer s.repoLock.RUnlock()
	for i, repo := range s.repos {
		if repo.Name == name {
			return repo, i
		}
	}
	return Repository{}, -1
}

func (s *GandalfServer) getFailure(method, path string) (Failure, bool) {
	var f Failure
	select {
	case f = <-s.failures:
		if f.Method == method && f.Path == path {
			return f, true
		}
		s.failures <- f
		return f, false
	default:
		return f, false
	}
}
