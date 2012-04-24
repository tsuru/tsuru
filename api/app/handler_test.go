package app_test

import (
	"bytes"
	"encoding/json"
	"github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *S) TestUpload(c *C) {
	fileApplicationContents, _ := ioutil.ReadFile("testdata/example.zip")
	message := `
--MyBoundary
Content-Disposition: form-data; name="application"; filename="application.zip"
Content-Type: application/zip

` + string(fileApplicationContents) + `
--MyBoundary--
`

	myApp := app.App{Name: "myApp", Framework: "django"}
	myApp.Create()

	b := bytes.NewBufferString(message)
	request, err := http.NewRequest("POST", "/apps"+myApp.Name+"/application?:name="+myApp.Name, b)
	c.Assert(err, IsNil)

	ctype := `multipart/form-data; boundary="MyBoundary"`
	request.Header.Set("Content-type", ctype)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = app.Upload(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 200)
	c.Assert(recorder.Body.String(), Equals, "success")

	myApp.Destroy()
}

func (s *S) TestUploadReturns404WhenAppDoesNotExist(c *C) {
	myApp := app.App{Name: "myAppThatDoestNotExists", Framework: "django"}
	request, err := http.NewRequest("POST", "/apps"+myApp.Name+"/application?:name="+myApp.Name, nil)
	c.Assert(err, IsNil)

	recorder := httptest.NewRecorder()
	err = app.Upload(recorder, request)
	c.Assert(err, IsNil)
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
	err = app.AppList(recorder, request)
	c.Assert(err, IsNil)
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
	err = app.AppDelete(recorder, request)
	c.Assert(err, IsNil)
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

	err = app.AppInfo(recorder, request)
	c.Assert(err, IsNil)
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
	err = app.AppInfo(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 404)
}

func (s *S) TestCreateApp(c *C) {
	b := strings.NewReader(`{"name":"someApp", "framework":"django"}`)
	request, err := http.NewRequest("POST", "/apps", b)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	c.Assert(err, IsNil)

	err = app.CreateAppHandler(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)

	collection := Db.C("apps")
	qtd, err := collection.Find(bson.M{"name": "someApp"}).Count()
	c.Assert(err, IsNil)
	c.Assert(qtd, Equals, 1)

	app := app.App{Name: "someApp"}
	app.Destroy()
}
