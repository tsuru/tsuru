package apps_test

//req {:method=>:post, :url=>"http://api.vcap.me/apps", :payload=>"{\"name\":\"app1\",\"staging\":{\"framework\":\"django\",\"runtime\":null},\"uris\":[\"app1.vcap.me\"],\"instances\":1,\"resources\":{\"memory\":128}}", :headers=>{"AUTHORIZATION"=>"04085b0849221c616e64726577736d6564696e6140676d61696c2e636f6d063a0645546c2b073b84704f2219c8524b951cd4b1e2574c826b6192cf911fa1a94f", "Content-Type"=>"application/json", "Accept"=>"application/json"}, :multipart=>true, :timeout=>86400000, :open_timeout=>86400000}

import (
	"github.com/timeredbull/tsuru/api/apps"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"fmt"
)

func (s *S) TestCreateApp(c *C) {
	request, err := http.NewRequest("POST", "/apps", nil)
	recorder := httptest.NewRecorder()

	c.Check(err, IsNil)
	apps.CreateAppHandler(recorder, request)
	fmt.Println(recorder.Code)

	c.Assert(recorder.Body.String(), Equals, "success")
	c.Assert(recorder.Code, Equals, 200)
}
