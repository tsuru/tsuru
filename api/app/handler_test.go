package app_test

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

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
