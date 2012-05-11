package auth

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/repository/gitosis"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io"
	"io/ioutil"
	"launchpad.net/mgo/bson"
	"net/http"
	"strings"
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

	u := User{Email: r.URL.Query().Get(":email")}
	err = u.Get()
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "User not found"}
	}

	if u.Login(password) {
		t, _ := u.CreateToken()
		fmt.Fprintf(w, `{"token":"%s"}`, t.Token)
		return nil
	}

	msg := "Authentication failed, wrong password"
	return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
}

func createTeam(name string, u *User) error {
	team := &Team{Name: name, Users: []*User{u}}
	err := db.Session.Teams().Insert(team)
	if err != nil && strings.Contains(err.Error(), "duplicate key error") {
		return &errors.Http{Code: http.StatusConflict, Message: "This team already exists"}
	}
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

func AddUserToTeam(w http.ResponseWriter, r *http.Request, u *User) error {
	team, user := new(Team), new(User)
	selector := bson.M{"name": r.URL.Query().Get(":team")}
	err := db.Session.Teams().Find(selector).One(team)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.ContainsUser(u) {
		msg := fmt.Sprintf("You are not authorized to add new users to the team %s", team.Name)
		return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
	}
	err = db.Session.Users().Find(bson.M{"email": r.URL.Query().Get(":user")}).One(user)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "User not found"}
	}
	err = team.AddUser(user)
	if err != nil {
		return &errors.Http{Code: http.StatusConflict, Message: err.Error()}
	}
	return db.Session.Teams().Update(selector, team)
}

func RemoveUserFromTeam(w http.ResponseWriter, r *http.Request, u *User) error {
	team := new(Team)
	selector := bson.M{"name": r.URL.Query().Get(":team")}
	err := db.Session.Teams().Find(selector).One(team)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.ContainsUser(u) {
		msg := fmt.Sprintf("You are not authorized to remove a member from the team %s", team.Name)
		return &errors.Http{Code: http.StatusUnauthorized, Message: msg}
	}
	if len(team.Users) == 1 {
		msg := "You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned"
		return &errors.Http{Code: http.StatusForbidden, Message: msg}
	}
	user := User{Email: r.URL.Query().Get(":user")}
	err = team.RemoveUser(&user)
	if err != nil {
		return &errors.Http{Code: http.StatusNotFound, Message: err.Error()}
	}
	return db.Session.Teams().Update(selector, team)
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
	filename, err := gitosis.BuildAndStoreKeyFile(u.Email, content)
	if err != nil {
		return err
	}
	key.Name = strings.Replace(filename, ".pub", "", -1)
	err = u.addKey(key)
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
	return gitosis.DeleteKeyFile(key.Name + ".pub")
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
