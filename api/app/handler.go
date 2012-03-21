package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func AppInfo(w http.ResponseWriter, r *http.Request) {
	var name string

	for _, token := range strings.Split(r.URL.Path, "/") {
		name = token
	}

	app := App{Name: name}
	app.Get()

	b, err := json.Marshal(app)
	if err != nil {
		panic(err)
	}

	fmt.Fprint(w, bytes.NewBuffer(b).String())
}

func CreateAppHandler(w http.ResponseWriter, r *http.Request) {
	var app App

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(body, &app)
	if err != nil {
		panic(err)
	}

	err = app.Create()
	if err != nil {
		panic(err)
	}
	fmt.Fprint(w, "success")
}
