package service

import (
	"fmt"
	"net/http"
	"encoding/json"
	"io/ioutil"
	. "github.com/timeredbull/tsuru/api/app"
)

type ServiceJson struct {
	Type string
	Name string
}

type BindJson struct {
	App     string
	Service string
}

func CreateHandler(w http.ResponseWriter, r *http.Request) {
	var sj ServiceJson

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(body, &sj)
	if err != nil {
		panic(err)
	}

	st := ServiceType{Name: sj.Type}
	st.Get()

	s := Service{
		Name:          sj.Name,
		ServiceTypeId: st.Id,
	}
	s.Create()
	fmt.Fprint(w, "success")
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	s := Service{Name: r.URL.Query().Get(":name")}
	s.Get()
	s.Delete()
	fmt.Fprint(w, "success")
}

func BindHandler(w http.ResponseWriter, r *http.Request) {
	var b BindJson

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(body, &b)
	if err != nil {
		panic(err)
	}

	s := Service{Name: b.Service}
	a := App{Name: b.App}
	s.Get()
	a.Get()

	fmt.Fprint(w, "success")
}
