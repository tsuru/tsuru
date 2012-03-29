package user

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func CreateUser(w http.ResponseWriter, r *http.Request) error {
	var u User
	b, _ := ioutil.ReadAll(r.Body)
	json.Unmarshal(b, &u)
	u.Create()
	return nil
}
