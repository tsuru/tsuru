package app_test

import (
	"bytes"
	"encoding/json"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *S) TestUpload(c *C) {
	fileApplicationContents := "This is a test file."
	message := `
--MyBoundary
Content-Disposition: form-data; name="application"; filename="application.txt"
Content-Type: text/plain

` + fileApplicationContents + `
--MyBoundary--
`

	myApp := app.App{Name: "myApp", Framework: "django"}
	myApp.Create()

	b := bytes.NewBufferString(strings.Replace(message, "\n", "\r\n", -1))
	request, err := http.NewRequest("POST", "/apps"+myApp.Name+"/application?:name="+myApp.Name, b)
	c.Assert(err, IsNil)

	ctype := `multipart/form-data; boundary="MyBoundary"`
	request.Header.Set("Content-type", ctype)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	app.Upload(recorder, request)

	c.Assert(recorder.Code, Equals, 200)
	c.Assert(recorder.Body.String(), Equals, "success")

	myApp.Destroy()
}

func (s *S) TestUploadReturns404WhenAppDoesNotExist(c *C) {
	myApp := app.App{Name: "myAppThatDoestNotExists", Framework: "django"}
	request, err := http.NewRequest("POST", "/apps"+myApp.Name+"/application?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	app.Upload(recorder, request)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *S) TestAppList(c *C) {
	apps := make([]app.App, 0)
	expected := make([]app.App, 0)
	app1 := app.App{Name: "app1"}
	app1.Create()
	expected = append(expected, app1)
	app2 := app.App{Name: "app2"}
	app2.Create()
	expected = append(expected, app2)
	app3 := app.App{Name: "app3", Framework: "django", Ip: "122222"}
	app3.Create()
	expected = append(expected, app3)

	request, err := http.NewRequest("GET", "/apps/", nil)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.AppList(recorder, request)
	c.Assert(recorder.Code, Equals, 200)

	body, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, IsNil)

	err = json.Unmarshal(body, &apps)
	c.Assert(err, IsNil)
	c.Assert(apps, DeepEquals, expected)

	app1.Destroy()
	app2.Destroy()
	app3.Destroy()
}

func (s *S) TestDelete(c *C) {
	myApp := app.App{Name: "MyAppToDelete", Framework: "django"}
	myApp.Create()
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	app.AppDelete(recorder, request)
	c.Assert(recorder.Code, Equals, 200)
}

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

func (s *S) TestAppInfoReturns404WhenAppDoesNotExist(c *C) {
	myApp := app.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)

	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.AppInfo(recorder, request)
	c.Assert(recorder.Code, Equals, 404)
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

	rows, err := s.db.Query("SELECT count(*) FROM apps WHERE name = 'someApp'")

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
