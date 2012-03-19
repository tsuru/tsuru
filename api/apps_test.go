package api_test

//req {:method=>:post, :url=>"http://api.vcap.me/apps", :payload=>"{\"name\":\"app1\",\"staging\":{\"framework\":\"django\",\"runtime\":null},\"uris\":[\"app1.vcap.me\"],\"instances\":1,\"resources\":{\"memory\":128}}", :headers=>{"AUTHORIZATION"=>"04085b0849221c616e64726577736d6564696e6140676d61696c2e636f6d063a0645546c2b073b84704f2219c8524b951cd4b1e2574c826b6192cf911fa1a94f", "Content-Type"=>"application/json", "Accept"=>"application/json"}, :multipart=>true, :timeout=>86400000, :open_timeout=>86400000}

import (
	"github.com/timeredbull/tsuru/api"
	. "launchpad.net/gocheck"
	"testing"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
)

func Test(t *testing.T) { TestingT(t)}

type WebServerSuite struct{}
var _ = Suite(&WebServerSuite{})

func (s *WebServerSuite) TestCreateApp(c *C) {
	request, err := http.NewRequest("POST", "/apps", nil)
	recorder := httptest.NewRecorder()

	c.Check(err, IsNil)
	api.CreateAppHandler(recorder, request)

	data, err := ioutil.ReadAll(recorder.Body)
	c.Assert(string(data), Equals, "success")
}
