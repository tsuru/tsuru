package provision

import (
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"io/ioutil"
	"net/http"
)

func AddDocHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := service.GetServiceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	s.Doc = string(body)
	if err = s.Update(); err != nil {
		return err
	}
	return nil
}

func GetDocHandler(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	s, err := service.GetServiceOrError(r.URL.Query().Get(":name"), u)
	if err != nil {
		return err
	}
	w.Write([]byte(s.Doc))
	return nil
}
