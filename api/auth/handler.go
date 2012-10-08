// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/validation"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strings"
)

const (
	emailError     = "Invalid email."
	passwordError  = "Password length shoul be least 6 characters and at most 50 characters."
	passwordMinLen = 6
	passwordMaxLen = 50
)

func CreateUser(w http.ResponseWriter, r *http.Request) error {
	var u User
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &u)
	if err != nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if !validation.ValidateEmail(u.Email) {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: emailError}
	}
	if !validation.ValidateLength(u.Password, passwordMinLen, passwordMaxLen) {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: passwordError}
	}
	err = u.Create()
	if err == nil {
		w.WriteHeader(http.StatusCreated)
		return nil
	}
	if u.Get() == nil {
		err = &errors.Http{Code: http.StatusConflict, Message: "This email is already registered"}
	}
	return err
}

func Login(w http.ResponseWriter, r *http.Request) error {
	var pass map[string]string
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &pass)
	if err != nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	password, ok := pass["password"]
	if !ok {
		msg := "You must provide a password to login"
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	if !validation.ValidateLength(password, passwordMinLen, passwordMaxLen) {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: passwordError}
	}
	u := User{Email: r.URL.Query().Get(":email")}
	if !validation.ValidateEmail(u.Email) {
		return &errors.Http{Code: http.StatusPreconditionFailed, Message: emailError}
	}
	err = u.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "User not found"}
	}
	if u.login(password) {
		t, _ := u.CreateToken()
		fmt.Fprintf(w, `{"token":"%s"}`, t.Token)
		return nil
	}
	msg := "Authentication failed, wrong password"
	return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
}

func applyChangesToKeys(kind int, team *Team, user *User) {
	for _, key := range user.Keys {
		log.Print("adding user ", key.Name, " to ", team.Name)
		ch := repository.Change{
			Kind: kind,
			Args: map[string]string{"group": team.Name, "member": key.Name},
		}
		repository.Ag.Process(ch)
	}
}

func createTeam(name string, u *User) error {
	team := &Team{Name: name, Users: []string{u.Email}}
	err := db.Session.Teams().Insert(team)
	if err != nil && strings.Contains(err.Error(), "duplicate key error") {
		return &errors.Http{Code: http.StatusConflict, Message: "This team already exists"}
	}
	ch := repository.Change{
		Kind:     repository.AddGroup,
		Args:     map[string]string{"group": name},
		Response: make(chan string),
	}
	repository.Ag.Process(ch)
	<-ch.Response
	applyChangesToKeys(repository.AddMember, team, u)
	return nil
}

func CreateTeam(w http.ResponseWriter, r *http.Request, u *User) error {
	var params map[string]string
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &params)
	if err != nil {
		return &errors.Http{Code: http.StatusBadRequest, Message: err.Error()}
	}
	name, ok := params["name"]
	if !ok {
		msg := "You must provide the team name"
		return &errors.Http{Code: http.StatusBadRequest, Message: msg}
	}
	return createTeam(name, u)
}

func ListTeams(w http.ResponseWriter, r *http.Request, u *User) error {
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	if len(teams) > 0 {
		var result []map[string]string
		for _, team := range teams {
			result = append(result, map[string]string{"name": team.Name})
		}
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n != len(b) {
			return &errors.Http{Code: http.StatusInternalServerError, Message: "Failed to write response body."}
		}
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
	return nil
}

func addUserToTeam(email, teamName string, u *User) error {
	team, user := new(Team), new(User)
	selector := bson.M{"_id": teamName}
	err := db.Session.Teams().Find(selector).One(team)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.containsUser(u) {
		msg := fmt.Sprintf("You are not authorized to add new users to the team %s", team.Name)
		return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
	}
	err = db.Session.Users().Find(bson.M{"email": email}).One(user)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "User not found"}
	}
	err = team.addUser(user)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	err = db.Session.Teams().Update(selector, team)
	if err != nil {
		return err
	}
	applyChangesToKeys(repository.AddMember, team, user)
	return nil
}

func AddUserToTeam(w http.ResponseWriter, r *http.Request, u *User) error {
	team := r.URL.Query().Get(":team")
	email := r.URL.Query().Get(":user")
	return addUserToTeam(email, team, u)
}

func removeUserFromTeam(email, teamName string, u *User) error {
	team := new(Team)
	selector := bson.M{"_id": teamName}
	err := db.Session.Teams().Find(selector).One(team)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.containsUser(u) {
		msg := fmt.Sprintf("You are not authorized to remove a member from the team %s", team.Name)
		return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
	}
	if len(team.Users) == 1 {
		msg := "You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	user := User{Email: email}
	err = user.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = team.removeUser(&user)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = db.Session.Teams().Update(selector, team)
	if err != nil {
		return err
	}
	applyChangesToKeys(repository.RemoveMember, team, &user)
	return nil
}

func RemoveUserFromTeam(w http.ResponseWriter, r *http.Request, u *User) error {
	email := r.URL.Query().Get(":user")
	team := r.URL.Query().Get(":team")
	return removeUserFromTeam(email, team, u)
}

func getKeyFromBody(b io.Reader) (string, error) {
	var body map[string]string
	content, err := ioutil.ReadAll(b)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(content, &body)
	if err != nil {
		return "", &errors.Http{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	key, ok := body["key"]
	if !ok || key == "" {
		return "", &errors.Http{Code: http.StatusBadRequest, Message: "Missing key"}
	}
	return key, nil
}

func addKeyToUser(content string, u *User) error {
	key := Key{Content: content}
	if u.hasKey(key) {
		return &errors.Http{Code: http.StatusConflict, Message: "User has this key already"}
	}
	r := make(chan string)
	ch := repository.Change{
		Kind:     repository.AddKey,
		Args:     map[string]string{"member": u.Email, "key": content},
		Response: r,
	}
	repository.Ag.Process(ch)
	var teams []Team
	db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	key.Name = strings.Replace(<-r, ".pub", "", -1)
	for _, team := range teams {
		mch := repository.Change{
			Kind: repository.AddMember,
			Args: map[string]string{"group": team.Name, "member": key.Name},
		}
		repository.Ag.Process(mch)
	}
	u.addKey(key)
	return db.Session.Users().Update(bson.M{"email": u.Email}, u)
}

// AddKeyToUser adds a key to a user.
//
// This function is just an http wrapper around addKeyToUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func AddKeyToUser(w http.ResponseWriter, r *http.Request, u *User) error {
	key, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	return addKeyToUser(key, u)
}

func removeKeyFromUser(content string, u *User) error {
	key, index := u.findKey(Key{Content: content})
	if index < 0 {
		return &errors.Http{Code: http.StatusNotFound, Message: "User does not have this key"}
	}
	u.removeKey(key)
	err := db.Session.Users().Update(bson.M{"email": u.Email}, u)
	if err != nil {
		return err
	}
	ch := repository.Change{
		Kind: repository.RemoveKey,
		Args: map[string]string{"key": key.Name + ".pub"},
	}
	repository.Ag.Process(ch)
	var teams []Team
	db.Session.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	for _, team := range teams {
		mch := repository.Change{
			Kind: repository.RemoveMember,
			Args: map[string]string{"group": team.Name, "member": key.Name},
		}
		repository.Ag.Process(mch)
	}
	return nil
}

// RemoveKeyFromUser removes a key from a user.
//
// This function is just an http wrapper around removeKeyFromUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func RemoveKeyFromUser(w http.ResponseWriter, r *http.Request, u *User) error {
	key, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	return removeKeyFromUser(key, u)
}
