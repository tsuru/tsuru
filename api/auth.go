// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/go-gandalfclient"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/errors"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/quota"
	"github.com/globocom/tsuru/rec"
	"github.com/globocom/tsuru/repository"
	"github.com/globocom/tsuru/validation"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"net/http"
)

const (
	// TODO(fss): move code that depend on these constants to package auth.
	emailError     = "Invalid email."
	passwordError  = "Password length should be least 6 characters and at most 50 characters."
	passwordMinLen = 6
	passwordMaxLen = 50
)

func createUser(w http.ResponseWriter, r *http.Request) error {
	var u auth.User
	err := json.NewDecoder(r.Body).Decode(&u)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if !validation.ValidateEmail(u.Email) {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: emailError}
	}
	if !validation.ValidateLength(u.Password, passwordMinLen, passwordMaxLen) {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: passwordError}
	}
	if _, err = auth.GetUserByEmail(u.Email); err == nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: "This email is already registered"}
	}
	gURL := repository.ServerURL()
	c := gandalf.Client{Endpoint: gURL}
	if _, err := c.NewUser(u.Email, keyToMap(u.Keys)); err != nil {
		return fmt.Errorf("Failed to create user in the git server: %s", err)
	}
	u.Quota = quota.Unlimited
	if limit, err := config.GetInt("quota:apps-per-user"); err == nil && limit > -1 {
		u.Quota.Limit = limit
	}
	if err := u.Create(); err == nil {
		rec.Log(u.Email, "create-user")
		w.WriteHeader(http.StatusCreated)
		return nil
	}
	return err
}

func login(w http.ResponseWriter, r *http.Request) error {
	var pass map[string]string
	err := json.NewDecoder(r.Body).Decode(&pass)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	password, ok := pass["password"]
	if !ok {
		msg := "You must provide a password to login"
		return &errors.HTTP{Code: http.StatusBadRequest, Message: msg}
	}
	u, err := auth.GetUserByEmail(r.URL.Query().Get(":email"))
	if err != nil {
		if e, ok := err.(*errors.ValidationError); ok {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: e.Message}
		} else if err == auth.ErrUserNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	rec.Log(u.Email, "login")
	t, err := u.CreateToken(password)
	if err != nil {
		switch err.(type) {
		case *errors.ValidationError:
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.(*errors.ValidationError).Message,
			}
		case auth.AuthenticationFailure:
			return &errors.HTTP{
				Code:    http.StatusUnauthorized,
				Message: err.Error(),
			}
		default:
			return err
		}
	}
	fmt.Fprintf(w, `{"token":"%s","is_admin":%v}`, t.Token, u.IsAdmin())
	return nil
}

func logout(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	auth.DeleteToken(t.Token)
	return nil
}

// ChangePassword changes the password from the logged in user.
//
// It reads the request body in JSON format. The JSON in the request body
// should contain two attributes:
//
// - old: the old password
// - new: the new password
//
// This handler will return 403 if the password didn't match the user, or 400
// if the new password is invalid.
func changePassword(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	var body map[string]string
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Invalid JSON.",
		}
	}
	if body["old"] == "" || body["new"] == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Both the old and the new passwords are required.",
		}
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	if err := u.CheckPassword(body["old"]); err != nil {
		return &errors.HTTP{
			Code:    http.StatusForbidden,
			Message: "The given password didn't match the user's current password.",
		}
	}
	if !validation.ValidateLength(body["new"], passwordMinLen, passwordMaxLen) {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: passwordError,
		}
	}
	rec.Log(u.Email, "change-password")
	u.Password = body["new"]
	u.HashPassword()
	return u.Update()
}

func resetPassword(w http.ResponseWriter, r *http.Request) error {
	email := r.URL.Query().Get(":email")
	token := r.URL.Query().Get("token")
	u, err := auth.GetUserByEmail(email)
	if err != nil {
		if err == auth.ErrUserNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		} else if e, ok := err.(*errors.ValidationError); ok {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: e.Error()}
		}
		return err
	}
	if token == "" {
		rec.Log(email, "reset-password-gen-token")
		return u.StartPasswordReset()
	}
	rec.Log(email, "reset-password")
	return u.ResetPassword(token)
}

// keyToMap converts a Key array into a map maybe we should store a map
// directly instead of having a convertion
func keyToMap(keys []auth.Key) map[string]string {
	kMap := make(map[string]string, len(keys))
	for _, k := range keys {
		kMap[k.Name] = k.Content
	}
	return kMap
}

func createTeam(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	var params map[string]string
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	name := params["name"]
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "create-team", name)
	err = auth.CreateTeam(name, u)
	switch err {
	case auth.ErrInvalidTeamName:
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	case auth.ErrTeamAlreadyExists:
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	return nil
}

// RemoveTeam removes a team document from the database.
func removeTeam(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	name := r.URL.Query().Get(":name")
	rec.Log(t.UserEmail, "remove-team", name)
	if n, err := conn.Apps().Find(bson.M{"teams": name}).Count(); err != nil || n > 0 {
		msg := `This team cannot be removed because it have access to apps.

Please remove the apps or revoke these accesses, and try again.`
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	query := bson.M{"_id": name, "users": t.UserEmail}
	err = conn.Teams().Remove(query)
	if err == mgo.ErrNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: fmt.Sprintf(`Team "%s" not found.`, name)}
	}
	return err
}

func teamList(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "list-teams")
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
			return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body."}
		}
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
	return nil
}

func addUserToTeamInDatabase(user *auth.User, team *auth.Team) error {
	if err := team.AddUser(user); err != nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Teams().UpdateId(team.Name, team)
}

func addUserToTeamInGandalf(user *auth.User, t *auth.Team) error {
	gURL := repository.ServerURL()
	alwdApps, err := t.AllowedApps()
	if err != nil {
		return fmt.Errorf("Failed to obtain allowed apps to grant: %s", err)
	}
	if err := (&gandalf.Client{Endpoint: gURL}).GrantAccess(alwdApps, []string{user.Email}); err != nil {
		return fmt.Errorf("Failed to grant access to git repositories: %s", err)
	}
	return nil
}

func addUserToTeam(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	teamName := r.URL.Query().Get(":team")
	email := r.URL.Query().Get(":user")
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "add-user-to-team", "team="+teamName, "user="+email)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	team, err := auth.GetTeam(teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.ContainsUser(u) {
		msg := fmt.Sprintf("You are not authorized to add new users to the team %s", team.Name)
		return &errors.HTTP{Code: http.StatusUnauthorized, Message: msg}
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "User not found"}
	}
	actions := []*action.Action{
		&addUserToTeamInGandalfAction,
		&addUserToTeamInDatabaseAction,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(user, team)
}

func removeUserFromTeamInDatabase(u *auth.User, team *auth.Team) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = team.RemoveUser(u); err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return conn.Teams().UpdateId(team.Name, team)
}

func removeUserFromTeamInGandalf(u *auth.User, team *auth.Team) error {
	gURL := repository.ServerURL()
	teamApps, err := team.AllowedApps()
	if err != nil {
		return err
	}
	userApps, err := u.AllowedApps()
	if err != nil {
		return err
	}
	appsToRemove := make([]string, 0, len(teamApps))
	for _, teamApp := range teamApps {
		found := false
		for _, userApp := range userApps {
			if userApp == teamApp {
				found = true
				break
			}
		}
		if !found {
			appsToRemove = append(appsToRemove, teamApp)
		}
	}
	client := gandalf.Client{Endpoint: gURL}
	if err := client.RevokeAccess(appsToRemove, []string{u.Email}); err != nil {
		return fmt.Errorf("Failed to revoke access from git repositories: %s", err)
	}
	return nil
}

func removeUserFromTeam(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	email := r.URL.Query().Get(":user")
	teamName := r.URL.Query().Get(":team")
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "remove-user-from-team", "team="+teamName, "user="+email)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	team, err := auth.GetTeam(teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.ContainsUser(u) {
		msg := fmt.Sprintf("You are not authorized to remove a member from the team %s", team.Name)
		return &errors.HTTP{Code: http.StatusUnauthorized, Message: msg}
	}
	if len(team.Users) == 1 {
		msg := "You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned"
		return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	err = removeUserFromTeamInDatabase(user, team)
	if err != nil {
		return err
	}
	return removeUserFromTeamInGandalf(user, team)
}

func getTeam(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	teamName := r.URL.Query().Get(":name")
	user, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(user.Email, "get-team", teamName)
	team, err := auth.GetTeam(teamName)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "Team not found"}
	}
	if !team.ContainsUser(user) {
		return &errors.HTTP{Code: http.StatusForbidden, Message: "User is not member of this team"}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(team)
}

func getKeyFromBody(b io.Reader) (string, error) {
	var body map[string]string
	err := json.NewDecoder(b).Decode(&body)
	if err != nil {
		return "", &errors.HTTP{Code: http.StatusBadRequest, Message: "Invalid JSON"}
	}
	key, ok := body["key"]
	if !ok || key == "" {
		return "", &errors.HTTP{Code: http.StatusBadRequest, Message: "Missing key"}
	}
	return key, nil
}

func addKeyInDatabase(key *auth.Key, u *auth.User) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	u.AddKey(*key)
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

func addKeyInGandalf(key *auth.Key, u *auth.User) error {
	key.Name = fmt.Sprintf("%s-%d", u.Email, len(u.Keys)+1)
	gURL := repository.ServerURL()
	if err := (&gandalf.Client{Endpoint: gURL}).AddKey(u.Email, keyToMap([]auth.Key{*key})); err != nil {
		return fmt.Errorf("Failed to add key to git server: %s", err)
	}
	return nil
}

// AddKeyToUser adds a key to a user.
//
// This function is just an http wrapper around addKeyToUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func addKeyToUser(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	content, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "add-key", content)
	key := auth.Key{Content: content}
	if u.HasKey(key) {
		return &errors.HTTP{Code: http.StatusConflict, Message: "User already has this key"}
	}
	actions := []*action.Action{
		&addKeyInGandalfAction,
		&addKeyInDatabaseAction,
	}
	pipeline := action.NewPipeline(actions...)
	return pipeline.Execute(&key, u)
}

func removeKeyFromDatabase(key *auth.Key, u *auth.User) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	u.RemoveKey(*key)
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

func removeKeyFromGandalf(key *auth.Key, u *auth.User) error {
	gURL := repository.ServerURL()
	if err := (&gandalf.Client{Endpoint: gURL}).RemoveKey(u.Email, key.Name); err != nil {
		return fmt.Errorf("Failed to remove the key from git server: %s", err)
	}
	return nil
}

// RemoveKeyFromUser removes a key from a user.
//
// This function is just an http wrapper around removeKeyFromUser. The latter function
// exists to be used in other places in the package without the http stuff (request and
// response).
func removeKeyFromUser(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	content, err := getKeyFromBody(r.Body)
	if err != nil {
		return err
	}
	u, err := t.User()
	if err != nil {
		return err
	}
	rec.Log(u.Email, "remove-key", content)
	key, index := u.FindKey(auth.Key{Content: content})
	if index < 0 {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "User does not have this key"}
	}
	err = removeKeyFromGandalf(&key, u)
	if err != nil {
		return err
	}
	return removeKeyFromDatabase(&key, u)
}

// listKeys list user's keys
func listKeys(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	keys, err := u.ListKeys()
	if err != nil {
		return err
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("Failed to marshal keys into json: %s", err)
	}
	n, err := w.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: "Failed to write response body."}
	}
	return nil
}

// removeUser removes the user from the database and from gandalf server
//
// If the user is the only one in a team an error will be returned.
func removeUser(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	u, err := t.User()
	if err != nil {
		return err
	}
	gURL := repository.ServerURL()
	c := gandalf.Client{Endpoint: gURL}
	alwdApps, err := u.AllowedApps()
	if err != nil {
		return err
	}
	if err := c.RevokeAccess(alwdApps, []string{u.Email}); err != nil {
		log.Errorf("Failed to revoke access in Gandalf: %s", err)
		return fmt.Errorf("Failed to revoke acess from git repositories: %s", err)
	}
	teams, err := u.Teams()
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, team := range teams {
		if len(team.Users) < 2 {
			msg := fmt.Sprintf(`This user is the last member of the team "%s", so it cannot be removed.

Please remove the team, then remove the user.`, team.Name)
			return &errors.HTTP{Code: http.StatusForbidden, Message: msg}
		}
		err = team.RemoveUser(u)
		if err != nil {
			return err
		}
		// this can be done without the loop
		err = conn.Teams().Update(bson.M{"_id": team.Name}, team)
		if err != nil {
			return err
		}
	}
	rec.Log(u.Email, "remove-user")
	if err := c.RemoveUser(u.Email); err != nil {
		log.Errorf("Failed to remove user from gandalf: %s", err)
		return fmt.Errorf("Failed to remove the user from the git server: %s", err)
	}
	return conn.Users().Remove(bson.M{"email": u.Email})
}

type jToken struct {
	Client string `json:"client"`
	Export bool   `json:"export"`
}

func generateAppToken(w http.ResponseWriter, r *http.Request, t *auth.Token) error {
	var body jToken
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return err
	}
	if body.Client == "" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "Missing client name in JSON body",
		}
	}
	token, err := auth.CreateApplicationToken(body.Client)
	if err != nil {
		return err
	}
	if body.Export {
		if a, err := app.GetByName(body.Client); err == nil {
			envs := []bind.EnvVar{
				{
					Name:   "TSURU_APP_TOKEN",
					Value:  token.Token,
					Public: false,
				},
			}
			a.SetEnvs(envs, false)
		}
	}
	return json.NewEncoder(w).Encode(token)
}
