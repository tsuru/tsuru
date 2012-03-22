package app_test

import (
	"database/sql"
	"encoding/json"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *S) TestAppInfo(c *C) {

	exptectedApp := app.App{Name: "NewApp", Framework: "django"}
	exptectedApp.Create()

	var myApp app.App

	request, err := http.NewRequest("GET", "/apps/"+exptectedApp.Name+"?:name="+exptectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c.Assert(err, IsNil)

	app.AppInfo(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	err = json.Unmarshal(body, &myApp)
	c.Assert(err, IsNil)
	c.Assert(myApp, Equals, exptectedApp)

	exptectedApp.Destroy()

}

func (s *S) TestCreateApp(c *C) {
	b := strings.NewReader(`{"name":"someApp", "framework":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	c.Assert(err, IsNil)

	app.CreateAppHandler(recorder, request)

	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)

	db, _ := sql.Open("sqlite3", "./tsuru.db")
	defer db.Close()
	rows, err := db.Query("SELECT count(*) FROM apps WHERE name = 'someApp'")

	if err != nil {
		panic(err)
	}

	var qtd int

	for rows.Next() {
		rows.Scan(&qtd)
	}

	c.Assert(qtd, Equals, 1)

	app := app.App{Name: "someApp"}
	app.Destroy()
}
