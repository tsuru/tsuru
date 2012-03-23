package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	. "github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	"net/http"
)

type ServiceJson struct {
	Type string
	Name string
}

type BindJson struct {
	App     string
	Service string
}

// a service with a pointer to it's type
type ServiceT struct {
	Id   int64
	Type *ServiceType
	Name string
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

	if s.Id == 0 {
		http.NotFound(w, r)
	}

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
	if s.Id == 0 || a.Id == 0 {
		http.NotFound(w, r)
	}

	s.Bind(&a)

	fmt.Fprint(w, "success")
}

func UnbindHandler(w http.ResponseWriter, r *http.Request) {
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
	if s.Id == 0 || a.Id == 0 {
		http.NotFound(w, r)
	}

	s.Unbind(&a)

	fmt.Fprint(w, "success")
}

func ServicesHandler(w http.ResponseWriter, r *http.Request) {
	services := All()
	results := make([]ServiceT, 0)

	var sT ServiceT
	for _, s := range services {
		sT = ServiceT{
			Id:   s.Id,
			Type: s.ServiceType(),
			Name: s.Name,
		}
		results = append(results, sT)
	}

	b, err := json.Marshal(results)
	if err != nil {
		panic(err)
	}

	fmt.Fprint(w, bytes.NewBuffer(b).String())
}
