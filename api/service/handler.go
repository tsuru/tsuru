package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

func CreateHandler(w http.ResponseWriter, r *http.Request) {
	var s Service

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(body, &s)
	if err != nil {
		panic(err)
	}

	st := ServiceType{Name: s.Type}
	st.Get()
	s.ServiceTypeId = st.Id
	s.Create()
	fmt.Fprint(w, "success")
}

func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	s := Service{Name: r.URL.Query().Get(":name")}
	s.Get()
	s.Delete()
	fmt.Fprint(w, "success")
}
