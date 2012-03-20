package app_test

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/api/app"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
)

func (s *S) TestCreateApp(c *C) {
	request, err := http.NewRequest("POST", "/apps", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Form = url.Values{"name": []string{"someApp"}, "framework": []string{"django"}}

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
