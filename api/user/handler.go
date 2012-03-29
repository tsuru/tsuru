package user

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func CreateUser(w http.ResponseWriter, r *http.Request) error {
	var u User
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &u)
	if err != nil {
		return err
	}
	err = u.Create()
	return err
}
