package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
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
	Type *ServiceType
	Name string
}

func ServicesHandler(w http.ResponseWriter, r *http.Request) error {
	s := Service{}
	services := s.All()
	results := make([]ServiceT, 0)

	var sT ServiceT
	for _, s := range services {
		sT = ServiceT{
			Type: s.ServiceType(),
			Name: s.Name,
		}
		results = append(results, sT)
	}

	b, err := json.Marshal(results)
	if err != nil {
		return err
	}

	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func ServiceTypesHandler(w http.ResponseWriter, r *http.Request) error {
	s := ServiceType{}
	sTypes := s.All()
	results := make([]ServiceType, 0)

	var sT ServiceType
	for _, s := range sTypes {
		sT = ServiceType{
			Id:    s.Id,
			Charm: s.Charm,
			Name:  s.Name,
		}
		results = append(results, sT)
	}

	b, err := json.Marshal(results)
	if err != nil {
		return err
	}

	fmt.Fprint(w, bytes.NewBuffer(b).String())
	return nil
}

func CreateHandler(w http.ResponseWriter, r *http.Request) error {
	var sj ServiceJson

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &sj)
	if err != nil {
		return err
	}

	st := ServiceType{Charm: sj.Type}
	st.Get()

	s := Service{
		Name:          sj.Name,
		ServiceTypeId: st.Id,
	}
	s.Create()
	fmt.Fprint(w, "success")
	return nil
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) error {
	s := Service{Name: r.URL.Query().Get(":name")}
	err := s.Get()

	if err != nil {
		http.NotFound(w, r)
		return err
	}
	s.Delete()
	fmt.Fprint(w, "success")

	return nil
}

func BindHandler(w http.ResponseWriter, r *http.Request) error {
	var b BindJson
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &b)
	if err != nil {
		return err
	}
	s := Service{Name: b.Service}
	a := app.App{Name: b.App}
	sErr := s.Get()
	aErr := a.Get()
	if sErr != nil || aErr != nil {
		http.NotFound(w, r)
	} else {
		s.Bind(&a)
		fmt.Fprint(w, "success")
	}
	return nil
}

func UnbindHandler(w http.ResponseWriter, r *http.Request) error {
	var b BindJson

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &b)
	if err != nil {
		return err
	}

	s := Service{Name: b.Service}
	a := app.App{Name: b.App}
	sErr := s.Get()
	aErr := a.Get()
	if sErr != nil || aErr != nil {
		http.NotFound(w, r)
	} else {
		s.Unbind(&a)
		fmt.Fprint(w, "success")
	}
	return nil
}
