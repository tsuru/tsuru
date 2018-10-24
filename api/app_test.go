// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ajg/form"
	"github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"github.com/tsuru/tsuru/router/rebuild"
	"github.com/tsuru/tsuru/router/routertest"
	"github.com/tsuru/tsuru/service"
	apiTypes "github.com/tsuru/tsuru/types/api"
	appTypes "github.com/tsuru/tsuru/types/app"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/types/cache"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	"gopkg.in/check.v1"
)

var (
	testCert = `-----BEGIN CERTIFICATE-----
MIIDkzCCAnugAwIBAgIJAIN09j/dhfmsMA0GCSqGSIb3DQEBCwUAMGAxCzAJBgNV
BAYTAkJSMRcwFQYDVQQIDA5SaW8gZGUgSmFuZWlybzEXMBUGA1UEBwwOUmlvIGRl
IEphbmVpcm8xDjAMBgNVBAoMBVRzdXJ1MQ8wDQYDVQQDDAZhcHAuaW8wHhcNMTcw
MTEyMjAzMzExWhcNMjcwMTEwMjAzMzExWjBgMQswCQYDVQQGEwJCUjEXMBUGA1UE
CAwOUmlvIGRlIEphbmVpcm8xFzAVBgNVBAcMDlJpbyBkZSBKYW5laXJvMQ4wDAYD
VQQKDAVUc3VydTEPMA0GA1UEAwwGYXBwLmlvMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEAw3GRuXOyL0Ar5BYA8DAPkY7ZHtHpEFK5bOoZB3lLBMjIbUKk
+riNTTgcY1eCsoAMZ0ZGmwmK/8mrJSBcsK/f1HVTcsSU0pA961ROPkAad/X/luSL
nXxDnZ1c0cOeU3GC4limB4CSZ64SZEDJvkUWnhUjTO4jfOCu0brkEnF8x3fpxfAy
OrAO50Uxij3VOQIAkP5B0T6x2Htr1ogm/vuubp5IG+KVuJHbozoaFFgRnDwrk+3W
k3FFUvg4ywY2jgJMLFJb0U3IIQgSqwQwXftKdu1EaoxA5fQmu/3a4CvYKKkwLJJ+
6L4O9Uf+QgaBZqTpDJ7XcIYbW+TPffzSwuI5PwIDAQABo1AwTjAdBgNVHQ4EFgQU
3XOK6bQW7hL47fMYH8JT/qCqIDgwHwYDVR0jBBgwFoAU3XOK6bQW7hL47fMYH8JT
/qCqIDgwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAgP4K9Zd1xSOQ
HAC6p2XjuveBI9Aswudaqg8ewYZtbtcbV70db+A69b8alSXfqNVqI4L2T97x/g6J
8ef8MG6TExhd1QktqtxtR+wsiijfUkityj8j5JT36TX3Kj0eIXrLJWxPEBhtGL17
ZBGdNK2/tDsQl5Wb+qnz5Ge9obybRLHHL2L5mrSwb+nC+nrC2nlfjJgVse9HhU9j
6Euq5hstXAlQH7fUbC5zAMS5UFrbzR+hOvjrSwzkkJmKW8BKKCfSaevRhq4VXxpw
Wx1oQV8UD5KLQQRy9Xew/KRHVzOpdkK66/i/hgV7GdREy4aKNAEBRpheOzjLDQyG
YRLI1QVj1Q==
-----END CERTIFICATE-----`

	testKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDDcZG5c7IvQCvk
FgDwMA+Rjtke0ekQUrls6hkHeUsEyMhtQqT6uI1NOBxjV4KygAxnRkabCYr/yasl
IFywr9/UdVNyxJTSkD3rVE4+QBp39f+W5IudfEOdnVzRw55TcYLiWKYHgJJnrhJk
QMm+RRaeFSNM7iN84K7RuuQScXzHd+nF8DI6sA7nRTGKPdU5AgCQ/kHRPrHYe2vW
iCb++65unkgb4pW4kdujOhoUWBGcPCuT7daTcUVS+DjLBjaOAkwsUlvRTcghCBKr
BDBd+0p27URqjEDl9Ca7/drgK9goqTAskn7ovg71R/5CBoFmpOkMntdwhhtb5M99
/NLC4jk/AgMBAAECggEBAJ6OlFqPsg8DUJhKAZjaZMcBzMNkKGBFvIjPol6d2G6Z
NYDugEmnT3tF+kHdzPpjR6zBJqbApzO8uEv2ZTwycrQ6Oujw8oug2ZsBWjjGaLLJ
sAEKiPnKxlAoShUjTl8Dx9s5b/jGJgBXCDStGv5xrlexbmILEF9PKISdyknsJ/7p
rLU+Oj8Ukus+PAr+2wr1DKyC6+FFHv7SF73ABEr/+IPIic590Ax36noaLz1XKcI2
AsAnFt6ThBwkH0x4BWPppyb4rS0h4QzUjUDs960uUce6P9Dp4Cy+Gl5l+FTaBcIL
hKUpHUkAId5ZBxusWuo9XADhXX9ujOP5XlZz8OYFeoECgYEA99alClsof+lPO1P/
kpHBgiAwR+4zZg6823AzWNDX1SbLHwB8rfRUlOydgwquNAfjmsA46SSfj/hQzQ8o
QH/3FrxLyY/wpnbSnJIzUMcKsalUyMUqoXDyQE5TK4SRo963zgdOOt2aFt5s1TpO
BNsJHEpq0mLe7seJ/WRzEAQPBJsCgYEAyeE4SKCr6UhMNc0v0zU76XCnB/b+C0mf
o9B0EsTOtDTx/NpXMq0DJ8+geVdxIXuKYs4c0avwIDVnk95EpRtMj7leEUOt9Dfb
M0ck4Z7sSae2LwQD8D0pni7NKn2kZjqbJQzu5R8bImQS1UQt8GFGQbZGXN+uDF15
FfjJINbA3i0CgYB/X69+vQ75fl0cLrWBDKwZRpXJwiBkaVqipO2ezea/Q6rNCiEJ
/jKiP2FMgea6EvvlArm9CPeAtKxCV3HmhF3nL2r78qBJzXO8yF7bOxDB8jcC4GJi
invWlOqlyQJY6BQrLRIFqvKQokvo4ohKcpAiHBT+f5X3vlGrCz8fkhZt1QKBgFFB
+RCqs2eLtTk2pNhjpgDZWjIHhcvvT3V1czMWyoiYgwqeq9h28T02Aka1HpE2k8Yf
ZlQy281rEYzgO0slyNRU7XsPfdY+IVnrefniqQMgoWEdQaSSSc0k02oV9nU7g7UP
Fp1cvuRB2Z7D+aW20bujbYD2e6z4dsOURwiTyD/lAoGAeRCGcPVJdNWJVpbq3WU/
JzxYPj5x/byRW+EMgHWxB1NTU+pINqp/IwKtPkU9UvjQJ0WgiYn4CKCkAQFY7LJt
AzzlaubwLUR9iKuJIh+wZioBT2jDqNTsN/UureuspGxu+RJaEUjL3NXN0KZ04sja
A/dGIKt8r4IkvjGdt2myS/A=
-----END PRIVATE KEY-----`
)

func (s *S) setupMockForCreateApp(c *check.C, platName string) {
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, platName)
		return &appTypes.Platform{Name: platName}, nil
	}
}

func (s *S) TestAppListFilteringByPlatform(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"a"}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "python", TeamOwner: s.team.Name, Tags: []string{"b", "c"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?platform=zend", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByTeamOwner(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"tag 1"}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: team.Name, Tags: []string{"tag 2"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?teamOwner=%s", s.team.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByOwner(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User())
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err := app.CreateApp(&app1, u)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "python", TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?owner=%s", u.Email), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByTags(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	u, _ := auth.ConvertNewUser(token.User())
	app1 := app.App{Name: "app1", TeamOwner: s.team.Name, Tags: []string{"tag1", "tag2"}}
	err := app.CreateApp(&app1, u)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", TeamOwner: s.team.Name, Tags: []string{"tag2", "tag3"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?tag=tag3", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app2.Name)
	c.Assert(apps[0].Tags, check.DeepEquals, app2.Tags)
	request, err = http.NewRequest("GET", "/apps?tag=tag2&tag=tag1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps = []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
}

func (s *S) TestAppListFilteringByLockState(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{
		Name:      "app2",
		Platform:  "python",
		TeamOwner: s.team.Name,
		Lock:      appTypes.AppLock{Locked: true},
		Tags:      []string{"mytag"},
	}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?locked=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByPool(c *check.C) {
	opts := []pool.AddPoolOptions{
		{Name: "pool1", Default: false, Public: true},
		{Name: "pool2", Default: false, Public: true},
	}
	for _, opt := range opts {
		err := pool.AddPool(opt)
		c.Assert(err, check.IsNil)
	}
	app1 := app.App{Name: "app1", Platform: "zend", Pool: opts[0].Name, TeamOwner: s.team.Name, Tags: []string{"mytag"}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", Pool: opts[1].Name, TeamOwner: s.team.Name, Tags: []string{""}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", fmt.Sprintf("/apps?pool=%s", opts[1].Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByStatus(c *check.C) {
	recorder := httptest.NewRecorder()
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	requestBody := strings.NewReader("units=2&process=web")
	request, err := http.NewRequest("PUT", "/apps/app1/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	request, err = http.NewRequest("POST", fmt.Sprintf("/apps/%s/stop", app1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	requestBody = strings.NewReader("units=1&process=web")
	request, err = http.NewRequest("PUT", "/apps/app2/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app3 := app.App{Name: "app3", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app3, s.user)
	c.Assert(err, check.IsNil)
	request, err = http.NewRequest("GET", "/apps?status=stopped&status=started", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app1, app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestAppListFilteringByStatusIgnoresInvalidValues(c *check.C) {
	recorder := httptest.NewRecorder()
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	requestBody := strings.NewReader("units=2&process=web")
	request, err := http.NewRequest("PUT", "/apps/app1/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	request, err = http.NewRequest("POST", fmt.Sprintf("/apps/%s/stop", app1.Name), nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name, Tags: []string{"tag"}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	requestBody = strings.NewReader("units=1&process=web")
	request, err = http.NewRequest("PUT", "/apps/app2/units", requestBody)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps?status=invalid&status=started", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	apps := []app.App{}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	expected := []app.App{app2}
	c.Assert(apps, check.HasLen, len(expected))
	for i, app := range apps {
		c.Assert(app.Name, check.DeepEquals, expected[i].Name)
		units, err := app.Units()
		c.Assert(err, check.IsNil)
		expectedUnits, err := expected[i].Units()
		c.Assert(err, check.IsNil)
		c.Assert(units, check.DeepEquals, expectedUnits)
		c.Assert(app.Tags, check.DeepEquals, expected[i].Tags)
	}
}

func (s *S) TestSimplifiedAppList(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app1 := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	acquireDate := time.Date(2015, time.February, 12, 12, 3, 0, 0, time.Local)
	app2 := app.App{
		Name:      "app2",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app2"},
		Pool:      "pool1",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "wanted",
			Owner:       s.user.Email,
			AcquireDate: acquireDate,
		},
		Tags: []string{"a"},
	}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps?simplified=true", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []app.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	app1u, _ := apps[0].Units()
	c.Assert(app1u, check.HasLen, 0)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
	c.Assert(app1u, check.HasLen, 0)
}

func (s *S) TestAppList(c *check.C) {
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app1 := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	acquireDate := time.Date(2015, time.February, 12, 12, 3, 0, 0, time.Local)
	app2 := app.App{
		Name:      "app2",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app2"},
		Pool:      "pool1",
		Lock: appTypes.AppLock{
			Locked:      true,
			Reason:      "wanted",
			Owner:       s.user.Email,
			AcquireDate: acquireDate,
		},
		Tags: []string{"a"},
	}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []app.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 2)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].CName, check.DeepEquals, app1.CName)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[0].Pool, check.Equals, app1.Pool)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
	c.Assert(apps[1].Name, check.Equals, app2.Name)
	c.Assert(apps[1].CName, check.DeepEquals, app2.CName)
	c.Assert(apps[1].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[1].Pool, check.Equals, app2.Pool)
	c.Assert(apps[1].Tags, check.DeepEquals, app2.Tags)

}

func (s *S) TestAppListAfterAppInfoHasAddr(c *check.C) {
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app1 := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		CName:     []string{"cname.app1"},
		Pool:      "pool1",
		Tags:      []string{},
	}
	err = app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/app1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []app.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].CName, check.DeepEquals, app1.CName)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "app1.fakerouter.com",
		Opts:    map[string]string{},
	}})
	c.Assert(apps[0].Pool, check.Equals, app1.Pool)
	c.Assert(apps[0].Tags, check.DeepEquals, app1.Tags)
}

func (s *S) TestAppListAfterAppInfoHasAddrLegacyRouter(c *check.C) {
	s.mockService.Cache.OnList = func(keys ...string) ([]cache.CacheEntry, error) {
		return []cache.CacheEntry{{Key: "app-router-addr\x00app1\x00fake", Value: "app1.fakerouter.com"}}, nil
	}
	p := pool.Pool{Name: "pool1"}
	opts := pool.AddPoolOptions{Name: p.Name, Public: true}
	err := pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	app1 := app.App{
		Name:      "app1",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Pool:      "pool1",
		Teams:     []string{s.team.Name},
		Router:    "fake",
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Apps().Insert(app1)
	c.Assert(err, check.IsNil)
	routertest.FakeRouter.AddBackend(&app1)
	request, err := http.NewRequest("GET", "/apps/app1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []app.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name:    "fake",
		Address: "app1.fakerouter.com",
		Opts:    map[string]string{},
	}})
}

func (s *S) TestAppListUnitsError(c *check.C) {
	app1 := app.App{
		Name:      "app1",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	provisiontest.ProvisionerInstance.PrepareFailure("Units", fmt.Errorf("some units error"))
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []struct {
		Name  string
		Units []provision.Unit
		Error string
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
	c.Assert(apps[0].Units, check.DeepEquals, []provision.Unit{})
	c.Assert(apps[0].Error, check.Equals, "unable to list app units: some units error")
}

func (s *S) TestAppListShouldListAllAppsOfAllTeamsThatTheUserHasPermission(c *check.C) {
	team := authTypes.Team{Name: "angra"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxTeam, team.Name),
	})
	u, _ := auth.ConvertNewUser(token.User())
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: "angra"}
	err := app.CreateApp(&app1, u)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, u)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var apps []app.App
	err = json.Unmarshal(recorder.Body.Bytes(), &apps)
	c.Assert(err, check.IsNil)
	c.Assert(apps, check.HasLen, 1)
	c.Assert(apps[0].Name, check.Equals, app1.Name)
}

func (s *S) TestListShouldReturnStatusNoContentWhenAppListIsNil(c *check.C) {
	request, err := http.NewRequest("GET", "/apps", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestDelete(c *check.C) {
	myApp := &app.App{
		Name:      "myapptodelete",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(myApp, s.user)
	c.Assert(err, check.IsNil)
	myApp, err = app.GetByName(myApp.Name)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole("deleter", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.delete")
	c.Assert(err, check.IsNil)
	err = s.user.AddRole("deleter", myApp.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(myApp.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.delete",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": myApp.Name},
		},
	}, eventtest.HasEvent)
	_, err = repository.Manager().GetRepository(myApp.Name)
	c.Assert(err, check.NotNil)
}

func (s *S) TestDeleteShouldReturnForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	myApp := app.App{Name: "app-to-delete", Platform: "zend"}
	err := s.conn.Apps().Insert(myApp)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppDelete,
		Context: permission.Context(permTypes.CtxApp, "-other-app-"),
	})
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestDeleteShouldReturnNotFoundIfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestDeleteAdminAuthorized(c *check.C) {
	myApp := &app.App{
		Name:      "myapptodelete",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(myApp, s.user)
	c.Assert(err, check.IsNil)
	myApp, err = app.GetByName(myApp.Name)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAppInfo(c *check.C) {
	config.Set("host", "http://myhost.com")
	expectedApp := app.App{Name: "new-app", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&expectedApp, s.user)
	c.Assert(err, check.IsNil)
	var myApp map[string]interface{}
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	role, err := permission.NewRole("reader", "app", "")
	c.Assert(err, check.IsNil)
	err = role.AddPermissions("app.read")
	c.Assert(err, check.IsNil)
	s.user.AddRole("reader", expectedApp.Name)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	err = json.Unmarshal(recorder.Body.Bytes(), &myApp)
	c.Assert(err, check.IsNil)
	c.Assert(myApp["name"], check.Equals, expectedApp.Name)
	c.Assert(myApp["repository"], check.Equals, "git@"+repositorytest.ServerHost+":"+expectedApp.Name+".git")
}

func (s *S) TestAppInfoReturnsForbiddenWhenTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	expectedApp := app.App{Name: "new-app", Platform: "zend"}
	err := s.conn.Apps().Insert(expectedApp)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/"+expectedApp.Name+"?:app="+expectedApp.Name, nil)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRead,
		Context: permission.Context(permTypes.CtxApp, "-other-app-"),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppInfoReturnsNotFoundWhenAppDoesNotExist(c *check.C) {
	myApp := app.App{Name: "SomeApp"}
	request, err := http.NewRequest("GET", "/apps/"+myApp.Name+"?:app="+myApp.Name, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App SomeApp not found.\n")
}

func (s *S) TestCreateAppRemoveRole(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	role, err := permission.NewRole("test", "team", "")
	c.Assert(err, check.IsNil)
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = user.AddRole(role.Name, "team")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Roles().RemoveId(role.Name)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateApp(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, token.GetUserName())
		return nil
	}
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppWithoutPlatform(c *check.C) {
	a := app.App{Name: "someapp"}
	data := "name=someapp"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
		},
	}, eventtest.HasEvent)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppTeamOwner(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	t1 := authTypes.Team{Name: "team1"}
	t2 := authTypes.Team{Name: "team2"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{t1, t2}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &t1, nil
	}
	permissions := []permission.Permission{
		{
			Scheme:  permission.PermAppCreate,
			Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: t1.Name},
		},
		{
			Scheme:  permission.PermAppCreate,
			Context: permTypes.PermissionContext{CtxType: permTypes.CtxTeam, Value: t2.Name},
		},
	}
	_, token := permissiontest.CustomUserWithPermission(c, nativeScheme, "anotheruser", permissions...)
	a := app.App{Name: "someapp"}
	data := "name=someapp&platform=zend&teamOwner=" + t1.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&gotApp)
	c.Assert(err, check.IsNil)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, check.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             appIP,
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{t1.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "teamOwner", "value": t1.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppAdminSingleTeam(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&gotApp)
	c.Assert(err, check.IsNil)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var appIP string
	appIP, err = s.provisioner.Addr(&gotApp)
	c.Assert(err, check.IsNil)
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             appIP,
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(obtained, check.DeepEquals, expected)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppCustomPlan(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	expectedPlan := appTypes.Plan{
		Name:     "myplan",
		Memory:   4194304,
		Swap:     5,
		CpuShare: 10,
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		c.Assert(name, check.Equals, expectedPlan.Name)
		return &expectedPlan, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{expectedPlan}, nil
	}
	data := "name=someapp&platform=zend&plan=myplan"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(gotApp.Plan, check.DeepEquals, expectedPlan)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "plan", "value": "myplan"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithDescription(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data, err := url.QueryUnescape("name=someapp&platform=zend&description=my app description")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": a.Name},
			{"name": "platform", "value": "zend"},
			{"name": "description", "value": "my app description"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithTags(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	data, err := url.QueryUnescape("name=someapp&platform=zend&tag=tag1&tag=tag2&tags.0=tag0")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":someapp.git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2"})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Kind:   "app.create",
		Owner:  token.GetUserName(),
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": "zend"},
			{"name": "tag", "value": []string{"tag1", "tag2"}},
			{"name": "tags.0", "value": "tag0"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithPool(c *check.C) {
	platName := "zend"
	s.setupMockForCreateApp(c, platName)
	err := pool.AddPool(pool.AddPoolOptions{Name: "mypool1", Public: true})
	c.Assert(err, check.IsNil)
	appName := "someapp"
	data, err := url.QueryUnescape(fmt.Sprintf("name=%s&platform=%s&pool=mypool1", appName, platName))
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + appName + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": appName}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(gotApp.Pool, check.Equals, "mypool1")
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": appName},
			{"name": "platform", "value": platName},
			{"name": "pool", "value": "mypool1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppWithRouter(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data, err := url.QueryUnescape("name=someapp&platform=zend&router=fake")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name: "fake",
		Opts: map[string]string{},
	}})
}

func (s *S) TestCreateAppWithRouterOpts(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "someapp"}
	data, err := url.QueryUnescape("name=someapp&platform=zend&routeropts.opt1=val1&routeropts.opt2=val2")
	c.Assert(err, check.IsNil)
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Routers, check.DeepEquals, []appTypes.AppRouter{{
		Name: "fake",
		Opts: map[string]string{"opt1": "val1", "opt2": "val2"},
	}})
}

func (s *S) TestCreateAppTwoTeams(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	team := authTypes.Team{Name: "tsurutwo"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(_ string) (*authTypes.Team, error) {
		return &team, nil
	}
	data := "name=someapp&platform=zend"
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a team to execute this action.\n")
}

func (s *S) TestCreateAppUserQuotaExceeded(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	s.mockService.UserQuota.OnInc = func(email string, q int) error {
		c.Assert(email, check.Equals, token.GetUserName())
		return &quota.QuotaExceededError{Available: 0, Requested: 1}
	}
	u, _ := token.User()
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.Users().Update(bson.M{"email": u.Email}, bson.M{"$set": bson.M{"quota": quota.Quota{Limit: 1, InUse: 1}}})
	b := strings.NewReader("name=someapp&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "Quota exceeded\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": "zend"},
		},
		ErrorMatches: `Quota exceeded`,
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppInvalidName(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	b := strings.NewReader("name=123myapp&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "Invalid app name, your app should have at most 40 " +
		"characters, containing only lower case letters, numbers " +
		"or dashes, starting with a letter."
	c.Assert(recorder.Body.String(), check.Equals, msg+"\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("123myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "123myapp"},
			{"name": "platform", "value": "zend"},
		},
		ErrorMatches: msg,
	}, eventtest.HasEvent)
}

func (s *S) TestCreateAppReturnsUnauthorizedIfNoPermissions(c *check.C) {
	s.setupMockForCreateApp(c, "django")
	token := userWithPermission(c)
	b := strings.NewReader("name=someapp&platform=django")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You don't have permission to do this action\n")
}

func (s *S) TestCreateAppReturnsConflictWithProperMessageWhenTheAppAlreadyExist(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "plainsofdawn", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("name=plainsofdawn&platform=zend")
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Matches, "tsuru failed to create the app \"plainsofdawn\": there is already an app with this name\n")
}

func (s *S) TestCreateAppWithDisabledPlatformAndPlatformUpdater(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermPlatformUpdate,
		Context: permission.Context(permTypes.CtxGlobal, ""),
	})
	p := appTypes.Platform{Name: "platDis", Disabled: true}
	s.setupMockForCreateApp(c, p.Name)
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, p.Name)
		return &p, nil
	}
	a := app.App{Name: "someapp"}
	data := "name=someapp&platform=" + p.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusCreated)
	repoURL := "git@" + repositorytest.ServerHost + ":" + a.Name + ".git"
	var obtained map[string]string
	expected := map[string]string{
		"status":         "success",
		"repository_url": repoURL,
		"ip":             "someapp.fakerouter.com",
	}
	err = json.Unmarshal(recorder.Body.Bytes(), &obtained)
	c.Assert(err, check.IsNil)
	c.Assert(obtained, check.DeepEquals, expected)
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "someapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Teams, check.DeepEquals, []string{s.team.Name})
	c.Assert(s.provisioner.GetUnits(&gotApp), check.HasLen, 0)
	u, _ := token.User()
	c.Assert(eventtest.EventDesc{
		Target: appTarget("someapp"),
		Owner:  u.Email,
		Kind:   "app.create",
		StartCustomData: []map[string]interface{}{
			{"name": "name", "value": "someapp"},
			{"name": "platform", "value": p.Name},
		},
	}, eventtest.HasEvent)
	_, err = repository.Manager().GetRepository(a.Name)
	c.Assert(err, check.IsNil)
}

func (s *S) TestCreateAppWithDisabledPlatformAndNotAdminUser(c *check.C) {
	p := appTypes.Platform{Name: "platDis", Disabled: true}
	s.setupMockForCreateApp(c, p.Name)
	s.mockService.Platform.OnFindByName = func(name string) (*appTypes.Platform, error) {
		c.Assert(name, check.Equals, p.Name)
		return &p, nil
	}
	data := "name=someapp&platform=" + p.Name
	b := strings.NewReader(data)
	request, err := http.NewRequest("POST", "/apps", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppCreate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid platform\n")
}

func (s *S) TestUpdateAppWithDescriptionOnly(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description=my app description")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Description, check.DeepEquals, "my app description")
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "description", "value": "my app description"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppPlatformOnly(c *check.C) {
	s.setupMockForCreateApp(c, "zend")
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	s.setupMockForCreateApp(c, "heimerdinger")
	b := strings.NewReader("platform=heimerdinger")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Platform, check.Equals, "heimerdinger")
	c.Assert(gotApp.UpdatePlatform, check.Equals, true)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "platform", "value": "heimerdinger"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppPlatformWithVersion(c *check.C) {
	s.setupMockForCreateApp(c, "myplatform")
	a := app.App{Name: "myapp", Platform: "myplatform", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	s.mockService.PlatformImage.OnFindImage = func(name, image string) (string, error) {
		c.Assert(name, check.Equals, "myplatform")
		c.Assert(image, check.Equals, "v1")
		return "tsuru/myplatform:v1", nil
	}
	b := strings.NewReader("platform=myplatform:v1")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.PlatformVersion, check.Equals, "v1")
	c.Assert(gotApp.UpdatePlatform, check.Equals, true)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "platform", "value": "myplatform:v1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithTagsOnly(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&tag=tag1&tag=tag2&tag=tag3&tags.0=tag0")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var gotApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "myapp"}).One(&gotApp)
	c.Assert(err, check.IsNil)
	c.Assert(gotApp.Tags, check.DeepEquals, []string{"tag0", "tag1", "tag2", "tag3"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update",
		StartCustomData: []map[string]interface{}{
			{"name": ":appname", "value": a.Name},
			{"name": "tag", "value": []string{"tag1", "tag2", "tag3"}},
			{"name": "tags.0", "value": "tag0"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUpdateAppWithTagsWithoutPermission(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateDescription,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("description1=s&tag=tag1&tag=tag2&tag=tag3")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUpdateAppImageReset(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("imageReset=true")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.UpdatePlatform, check.Equals, true)
}

func (s *S) TestUpdateAppWithPoolOnly(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("test", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppPoolForbiddenIfTheUserDoesNotHaveAccess(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend"}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "test"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdatePool,
		Context: permission.Context(permTypes.CtxApp, "-other-"),
	})
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUpdateAppPoolWhenAppDoesNotExist(c *check.C) {
	body := strings.NewReader("pool=test")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "^App not found\n$")
}

func (s *S) TestUpdateAppPoolWithDifferentProvisioner(c *check.C) {
	p1 := provisiontest.NewFakeProvisioner()
	p1.Name = "fake1"
	provision.Register("fake1", func() (provision.Provisioner, error) {
		return p1, nil
	})
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	opts := pool.AddPoolOptions{Name: "fakepool", Provisioner: "fake1"}
	err = pool.AddPool(opts)
	c.Assert(err, check.IsNil)
	err = pool.AddTeamsToPool("fakepool", []string{s.team.Name})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("pool=fakepool")
	request, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppPlanOnly(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	plans := []appTypes.Plan{
		{Name: "hiperplan", Memory: 536870912, Swap: 536870912, CpuShare: 100},
		{Name: "superplan", Memory: 268435456, Swap: 268435456, CpuShare: 100},
	}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plans[0].Name {
			return &plans[0], nil
		}
		if name == plans[1].Name {
			return &plans[1], nil
		}
		c.Errorf("plan name not expected, got: %s", name)
		return nil, nil
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return plans, nil
	}
	a := app.App{Name: "someapp", Platform: "zend", TeamOwner: s.team.Name, Plan: plans[1]}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("plan=hiperplan")
	request, err := http.NewRequest("PUT", "/apps/someapp", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Plan, check.DeepEquals, plans[0])
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 1)
}

func (s *S) TestUpdateAppPlanNotFound(c *check.C) {
	plan := appTypes.Plan{Name: "superplan", Memory: 268435456, Swap: 268435456, CpuShare: 100}
	s.mockService.Plan.OnFindByName = func(name string) (*appTypes.Plan, error) {
		if name == plan.Name {
			return &plan, nil
		}
		return nil, appTypes.ErrPlanNotFound
	}
	s.mockService.Plan.OnList = func() ([]appTypes.Plan, error) {
		return []appTypes.Plan{plan}, nil
	}
	a := app.App{Name: "someapp", Platform: "zend", TeamOwner: s.team.Name, Plan: plan}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("plan=hiperplan")
	request, err := http.NewRequest("PUT", "/apps/someapp", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Check(recorder.Body.String(), check.Equals, appTypes.ErrPlanNotFound.Error()+"\n")
}

func (s *S) TestUpdateAppWithoutFlag(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader("{}")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	errorMessage := "Neither the description, tags, plan, pool, team owner or platform were set. You must define at least one.\n"
	c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Check(recorder.Body.String(), check.Equals, errorMessage)
}

func (s *S) TestUpdateAppReturnsUnauthorizedIfNoPermissions(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	b := strings.NewReader("description=description of my app")
	request, err := http.NewRequest("PUT", "/apps/myapp", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, 403)
}

func (s *S) TestUpdateAppWithTeamOwnerOnly(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateTeamowner,
		Context: permission.Context(permTypes.CtxTeam, a.TeamOwner),
	})
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "newowner"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{team, {Name: s.team.Name}}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	body := strings.NewReader("teamOwner=" + team.Name)
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
}

func (s *S) TestUpdateAppTeamOwnerToUserWhoCantBeOwner(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	user := &auth.User{Email: "teste@thewho.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err = nativeScheme.Create(user)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.Login(map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	body := strings.NewReader("teamOwner=newowner")
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusForbidden)
	s.conn.Apps().Find(bson.M{"name": "myappx"}).One(&a)
	c.Assert(a.TeamOwner, check.Equals, s.team.Name)
}

func (s *S) TestUpdateAppTeamOwnerSetNewTeamToAppAddThatTeamToAppTeamList(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateTeamowner,
		Context: permission.Context(permTypes.CtxTeam, a.TeamOwner),
	})
	user, err := auth.ConvertNewUser(token.User())
	c.Assert(err, check.IsNil)
	err = app.CreateApp(&a, user)
	c.Assert(err, check.IsNil)
	team := authTypes.Team{Name: "newowner"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, team}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, team.Name)
		return &team, nil
	}
	body := strings.NewReader("teamOwner=" + team.Name)
	req, err := http.NewRequest("PUT", "/apps/myappx", body)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "bearer "+token.GetValue())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.testServer.ServeHTTP(rec, req)
	c.Assert(rec.Code, check.Equals, http.StatusOK)
}

func (s *S) TestAddUnits(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 10, InUse: 0}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"added 3 units"}`+"\n")
}

func (s *S) TestAddUnitsUnlimited(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.UnlimitedQuota}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"added 3 units"}`+"\n")
}

func (s *S) TestAddUnitsReturns404IfAppDoesNotExist(c *check.C) {
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App armorandsword not found.\n")
}

func (s *S) TestAddUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("units=1&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateUnitAdd,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddUnitsReturns400IfNumberOfUnitsIsOmitted(c *check.C) {
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "You must provide the number of units.")
	}
}

func (s *S) TestAddUnitsWorksIfProcessIsOmitted(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.UnlimitedQuota}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return nil
	}
	body := strings.NewReader("units=3&process=")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = addUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"added 3 units"}`+"\n")
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": ""}, {"name": ":app", "value": "armorandsword"}},
	}, eventtest.HasEvent)
}

func (s *S) TestAddUnitsReturns400IfNumberIsInvalid(c *check.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		body := strings.NewReader("units=" + value + "&process=web")
		request, err := http.NewRequest("PUT", "/apps/armorandsword/units?:app=armorandsword", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = addUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestAddUnitsQuotaExceeded(c *check.C) {
	a := app.App{Name: "armorandsword", Platform: "zend", TeamOwner: s.team.Name, Quota: quota.Quota{Limit: 2, InUse: 0}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 3)
		return &quota.QuotaExceededError{Available: 2, Requested: 3}
	}
	body := strings.NewReader("units=3&process=web")
	request, err := http.NewRequest("PUT", "/apps/armorandsword/units", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, `Quota exceeded. Available: 2, Requested: 3.`+"\n")
	c.Assert(eventtest.EventDesc{
		Target:          appTarget("armorandsword"),
		Owner:           s.token.GetUserName(),
		Kind:            "app.update.unit.add",
		StartCustomData: []map[string]interface{}{{"name": "units", "value": "3"}, {"name": "process", "value": "web"}, {"name": ":app", "value": "armorandsword"}},
		ErrorMatches:    `Quota exceeded. Available: 2, Requested: 3.`,
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveUnits(c *check.C) {
	a := app.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 2)
		return nil
	}
	s.provisioner.AddUnits(&a, 3, "web", nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?units=2&process=web", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app), check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("velha"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unit.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "units", "value": "2"},
			{"name": "process", "value": "web"},
			{"name": ":app", "value": "velha"},
		},
	}, eventtest.HasEvent)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"removing 2 units"}`+"\n")
}

func (s *S) TestRemoveUnitsReturns404IfAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha&units=1&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, "App fetisha not found.")
}

func (s *S) TestRemoveUnitsReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "fetisha", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateUnitRemove,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha&units=1&process=web", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveUnitsReturns400IfNumberOfUnitsIsOmitted(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/fetisha/units?:app=fetisha", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "You must provide the number of units.")
}

func (s *S) TestRemoveUnitsWorksIfProcessIsOmitted(c *check.C) {
	a := app.App{Name: "velha", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.AppQuota.OnInc = func(appName string, quantity int) error {
		c.Assert(appName, check.Equals, a.Name)
		c.Assert(quantity, check.Equals, 2)
		return nil
	}
	s.provisioner.AddUnits(&a, 3, "", nil)
	request, err := http.NewRequest("DELETE", "/apps/velha/units?:app=velha&units=2&process=", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = removeUnits(recorder, request, s.token)
	c.Assert(err, check.IsNil)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	units, err := app.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	c.Assert(s.provisioner.GetUnits(app), check.HasLen, 1)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("velha"),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unit.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "units", "value": "2"},
			{"name": "process", "value": ""},
			{"name": ":app", "value": "velha"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveUnitsReturns400IfNumberIsInvalid(c *check.C) {
	values := []string{"-1", "0", "far cry", "12345678909876543"}
	for _, value := range values {
		v := url.Values{
			":app":    []string{"fiend"},
			"units":   []string{value},
			"process": []string{"web"},
		}
		request, err := http.NewRequest("DELETE", "/apps/fiend/units?"+v.Encode(), nil)
		c.Assert(err, check.IsNil)
		recorder := httptest.NewRecorder()
		err = removeUnits(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Assert(e.Code, check.Equals, http.StatusBadRequest)
		c.Assert(e.Message, check.Equals, "Invalid number of units: the number must be an integer greater than 0.")
	}
}

func (s *S) TestSetUnitStatus(c *check.C) {
	a := app.App{Name: "telegram", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	body := strings.NewReader("status=error")
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	unit := units[0]
	request, err := http.NewRequest("POST", "/apps/telegram/units/"+unit.ID, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	unit = units[0]
	c.Assert(unit.Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetUnitStatusNoUnit(c *check.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "missing unit")
}

func (s *S) TestSetUnitStatusInvalidStatus(c *check.C) {
	bodies := []io.Reader{strings.NewReader("status=something"), strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		err = setUnitStatus(recorder, request, s.token)
		c.Assert(err, check.NotNil)
		e, ok := err.(*errors.HTTP)
		c.Assert(ok, check.Equals, true)
		c.Check(e.Code, check.Equals, http.StatusBadRequest)
		c.Check(e.Message, check.Equals, provision.ErrInvalidStatus.Error())
	}
}

func (s *S) TestSetUnitStatusAppNotFound(c *check.C) {
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/velha/units/af32db?:app=velha&:unit=af32db", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	err = setUnitStatus(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Check(e.Code, check.Equals, http.StatusNotFound)
	c.Check(e.Message, check.Equals, "App not found")
}

func (s *S) TestSetUnitStatusDoesntRequireLock(c *check.C) {
	a := app.App{Name: "telegram", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	locked, err := app.AcquireApplicationLock(a.Name, "test", "test")
	c.Assert(err, check.IsNil)
	c.Assert(locked, check.Equals, true)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	unit := units[0]
	body := strings.NewReader("status=error")
	request, err := http.NewRequest("POST", "/apps/telegram/units/"+unit.ID, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	unit = units[0]
	c.Assert(unit.Status, check.Equals, provision.StatusError)
}

func (s *S) TestSetNodeStatus(c *check.C) {
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "telegram", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "addr1",
	})
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.AddUnitsToNode(&a, 3, "web", nil, "addr1")
	c.Assert(err, check.IsNil)
	status := []string{"started", "error", "stopped"}
	unitsStatus := []provision.UnitStatusData{
		{ID: units[0].ID, Status: "started"},
		{ID: units[1].ID, Status: "error"},
		{ID: units[2].ID, Status: "stopped"},
		{ID: "not-found1", Status: "error"},
		{ID: "not-found2", Status: "started"},
	}
	nodeStatus := provision.NodeStatusData{Addrs: []string{"addr1"}, Units: unitsStatus}
	v, err := form.EncodeToValues(&nodeStatus)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/node/status", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	for i, unit := range units {
		c.Check(unit.Status, check.Equals, provision.Status(status[i]))
	}
	var got updateList
	expected := updateList([]app.UpdateUnitsResult{
		{ID: units[0].ID, Found: true},
		{ID: units[1].ID, Found: true},
		{ID: units[2].ID, Found: true},
		{ID: "not-found1", Found: false},
		{ID: "not-found2", Found: false},
	})
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	sort.Sort(&got)
	sort.Sort(&expected)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestSetNodeStatusNotFound(c *check.C) {
	token, err := nativeScheme.AppLogin(app.InternalAppName)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "telegram", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 3, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.AddUnitsToNode(&a, 3, "web", nil, "addr1")
	c.Assert(err, check.IsNil)
	unitsStatus := []provision.UnitStatusData{
		{ID: units[0].ID, Status: "started"},
		{ID: units[1].ID, Status: "error"},
		{ID: units[2].ID, Status: "stopped"},
		{ID: "not-found1", Status: "error"},
		{ID: "not-found2", Status: "started"},
	}
	nodeStatus := provision.NodeStatusData{Addrs: []string{"addr1"}, Units: unitsStatus}
	v, err := form.EncodeToValues(&nodeStatus)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/node/status", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var got updateList
	expected := updateList([]app.UpdateUnitsResult{
		{ID: units[0].ID, Found: false},
		{ID: units[1].ID, Found: false},
		{ID: units[2].ID, Found: false},
		{ID: "not-found1", Found: false},
		{ID: "not-found2", Found: false},
	})
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	sort.Sort(&got)
	sort.Sort(&expected)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestSetNodeStatusNonInternalToken(c *check.C) {
	body := bytes.NewBufferString("{{{-")
	request, err := http.NewRequest("POST", "/node/status", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

type updateList []app.UpdateUnitsResult

func (list updateList) Len() int {
	return len(list)
}

func (list updateList) Less(i, j int) bool {
	return list[i].ID < list[j].ID
}

func (list updateList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (s *S) TestAddTeamToTheApp(c *check.C) {
	t := authTypes.Team{Name: "itshardteam"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{{Name: s.team.Name}, t}, nil
	}
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		return &authTypes.Team{Name: name}, nil
	}
	a := app.App{Name: "itshard", Platform: "zend", TeamOwner: t.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.HasLen, 2)
	c.Assert(app.Teams[1], check.Equals, s.team.Name)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.grant",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("PUT", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestGrantAccessToTeamReturn403IfTheGivenUserDoesNotHasAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateGrant,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGrantAccessToTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	newTeamName := "newteam"
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, newTeamName)
		return nil, authTypes.ErrTeamNotFound
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, newTeamName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestGrantAccessToTeamReturn409IfTheTeamHasAlreadyAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.grant",
		ErrorMatches: "team already have access to this app",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestGrantAccessToTeamCallsRepositoryManager(c *check.C) {
	t := authTypes.Team{Name: "anything"}
	s.mockService.Team.OnList = func() ([]authTypes.Team, error) {
		return []authTypes.Team{t, {Name: s.team.Name}}, nil
	}
	a := app.App{
		Name:      "tsuru",
		Platform:  "zend",
		TeamOwner: t.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRevokeAccessFromTeam(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{"abcd", s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.Teams, check.HasLen, 1)
	c.Assert(app.Teams[0], check.Equals, "abcd")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.revoke",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/a/teams/b", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App a not found.\n")
}

func (s *S) TestRevokeAccessFromTeamReturn401IfTheGivenUserDoesNotHavePermissionInTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRevoke,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotExist(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	teamName := "notfound"
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, teamName)
		return nil, authTypes.ErrTeamNotFound
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, teamName)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Team not found\n")
}

func (s *S) TestRevokeAccessFromTeamReturn404IfTheTeamDoesNotHaveAccessToTheApp(c *check.C) {
	t := authTypes.Team{Name: "blaaa"}
	t2 := authTypes.Team{Name: "team2"}
	a := app.App{Name: "itshard", Platform: "zend", Teams: []string{s.team.Name, t2.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, t.Name)
		return &t, nil
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRevokeAccessFromTeamReturn403IfTheTeamIsTheLastWithAccessToTheApp(c *check.C) {
	a := app.App{Name: "itshard", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, s.team.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	c.Assert(recorder.Body.String(), check.Equals, "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.revoke",
		ErrorMatches: "You can not revoke the access from this team, because it is the unique team with access to the app, and an app can not be orphaned",
		StartCustomData: []map[string]interface{}{
			{"name": ":team", "value": s.team.Name},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRevokeAccessFromTeamRemovesRepositoryFromRepository(c *check.C) {
	t := authTypes.Team{Name: "any-team"}
	newToken := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppDeploy,
		Context: permission.Context(permTypes.CtxTeam, t.Name),
	})
	a := app.App{Name: "tsuru", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, t.Name)
		return &t, nil
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email, newToken.GetUserName()})
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err = repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRevokeAccessFromTeamDontRemoveTheUserIfItHasAccesToTheAppThroughAnotherTeam(c *check.C) {
	u := auth.User{Email: "burning@angel.com", Quota: quota.UnlimitedQuota}
	err := s.conn.Users().Insert(u)
	c.Assert(err, check.IsNil)
	repository.Manager().CreateUser(u.Email)
	t := authTypes.Team{Name: "anything"}
	a := app.App{Name: "tsuru", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.mockService.Team.OnFindByName = func(name string) (*authTypes.Team, error) {
		c.Assert(name, check.Equals, t.Name)
		return &t, nil
	}
	url := fmt.Sprintf("/apps/%s/teams/%s", a.Name, t.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	request, err = http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	grants, err := repositorytest.Granted(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(grants, check.DeepEquals, []string{s.user.Email})
}

func (s *S) TestRunOnce(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls&once=true"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"lots of files"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 3)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": "once", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRun(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of\nfiles"))
	a := app.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"lots of\nfiles"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	units, err := a.GetUnits()
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()], check.HasLen, 1)
	c.Assert(allExecs[units[0].GetID()][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunIsolated(c *check.C) {
	s.provisioner.PrepareOutput([]byte("lots of files"))
	a := app.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 3, "web", nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls&isolated=true"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, `{"Message":"lots of files"}`+"\n")
	expected := "[ -f /home/application/apprc ] && source /home/application/apprc;"
	expected += " [ -d /home/application/current ] && cd /home/application/current;"
	expected += " ls"
	allExecs := s.provisioner.AllExecs()
	c.Assert(allExecs, check.HasLen, 1)
	c.Assert(allExecs["isolated"], check.HasLen, 1)
	c.Assert(allExecs["isolated"][0].Cmds, check.DeepEquals, []string{"/bin/sh", "-c", expected})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.run",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": "isolated", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunReturnsTheOutputOfTheCommandEvenIfItFails(c *check.C) {
	s.provisioner.PrepareFailure("ExecuteCommand", &errors.HTTP{Code: 500, Message: "something went wrong"})
	s.provisioner.PrepareOutput([]byte("failure output"))
	a := app.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	expected := `{"Message":"failure output"}` + "\n" +
		`{"Message":"","Error":"something went wrong"}` + "\n"
	c.Assert(recorder.Body.String(), check.Equals, expected)
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.run",
		ErrorMatches: "something went wrong",
		StartCustomData: []map[string]interface{}{
			{"name": "command", "value": "ls"},
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRunReturnsBadRequestIfTheCommandIsMissing(c *check.C) {
	a := app.App{Name: "secrets", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, body := range bodies {
		request, err := http.NewRequest("POST", "/apps/secrets/run", body)
		c.Assert(err, check.IsNil)
		request.Header.Set("content-type", "application/x-www-form-urlencoded")
		request.Header.Set("authorization", "b "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Assert(recorder.Body.String(), check.Equals, "You must provide the command to run\n")
	}
}

func (s *S) TestRunAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/unknown/run", strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestRunUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "secrets", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppRun,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/run", a.Name)
	request, err := http.NewRequest("POST", url, strings.NewReader("command=ls"))
	c.Assert(err, check.IsNil)
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	request.Header.Set("authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvAllEnvs(c *check.C) {
	a := app.App{
		Name:      "everything-i-want",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST": {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER": {Name: "DATABASE_USER", Value: "root", Public: true},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?envs=", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := []bind.EnvVar{
		{Name: "DATABASE_HOST", Value: "localhost", Public: true},
		{Name: "DATABASE_USER", Value: "root", Public: true},
		{Name: "TSURU_APPNAME", Value: "everything-i-want", Public: false},
		{Name: "TSURU_APPDIR", Value: "/home/application/current", Public: false},
		{Name: "TSURU_APP_TOKEN", Value: "123", Public: false},
		{Name: "TSURU_SERVICES", Value: "{}", Public: false},
	}
	result := []bind.EnvVar{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(len(result), check.Equals, len(expected))
	for _, r := range result {
		if r.Name == "TSURU_APP_TOKEN" {
			continue
		}
		for _, e := range expected {
			if e.Name == r.Name {
				c.Check(e.Public, check.Equals, r.Public)
				c.Check(e.Value, check.Equals, r.Value)
			}
		}
	}
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetEnv(c *check.C) {
	a := app.App{
		Name:      "everything-i-want",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGetEnvMultipleVariables(c *check.C) {
	a := app.App{
		Name:      "four-sticks",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?env=DATABASE_HOST&env=DATABASE_USER", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-type"), check.Equals, "application/json")
	expected := []map[string]interface{}{
		{"name": "DATABASE_HOST", "value": "localhost", "public": true},
		{"name": "DATABASE_USER", "value": "root", "public": true},
	}
	var got []map[string]interface{}
	err = json.Unmarshal(recorder.Body.Bytes(), &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestGetEnvAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/env", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestGetEnvUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadEnv,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/env?envs=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestGetEnvWithAppToken(c *check.C) {
	a := app.App{
		Name:      "everything-i-want",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	token, err := nativeScheme.AppLogin(a.Name)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	expected := []map[string]interface{}{{
		"name":   "DATABASE_HOST",
		"value":  "localhost",
		"public": true,
	}}
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestSetEnvPublicEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetAPrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetADoublePrivateEnvironmentVariableInTheApp(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
	d = apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "127.0.0.1"},
			{"DATABASE_PORT", "6379"},
		},
		NoRestart: false,
		Private:   true,
	}
	v, err = form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b = strings.NewReader(v.Encode())
	request, err = http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "127.0.0.1", Public: false}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 2 new environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "*****"},
			{"name": "Envs.1.Name", "value": "DATABASE_PORT"},
			{"name": "Envs.1.Value", "value": "*****"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldSetMultipleEnvironmentVariablesInTheApp(c *check.C) {
	a := app.App{Name: "vigil", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
			{"DATABASE_USER", "root"},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("vigil")
	c.Assert(err, check.IsNil)
	expectedHost := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	expectedUser := bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expectedHost)
	c.Assert(app.Env["DATABASE_USER"], check.DeepEquals, expectedUser)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "Envs.1.Name", "value": "DATABASE_USER"},
			{"name": "Envs.1.Value", "value": "root"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerShouldNotChangeValueOfServiceVariables(c *check.C) {
	a := &app.App{Name: "losers", Platform: "zend", Teams: []string{s.team.Name}, ServiceEnvs: []bind.ServiceEnvVar{
		{
			EnvVar: bind.EnvVar{
				Name:  "DATABASE_HOST",
				Value: "privatehost.com",
			},
			ServiceName:  "srv1",
			InstanceName: "some service",
		},
	}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "http://foo.com:8080"},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	a, err = app.GetByName("losers")
	c.Assert(err, check.IsNil)
	envs := a.Envs()
	delete(envs, app.TsuruServicesEnvVar)
	expected := map[string]bind.EnvVar{
		"DATABASE_HOST": {
			Name:  "DATABASE_HOST",
			Value: "privatehost.com",
		},
	}
	c.Assert(envs, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "http://foo.com:8080"},
			{"name": "NoRestart", "value": ""},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvHandlerNoRestart(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
		},
		NoRestart: true,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("black-dog")
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{Name: "DATABASE_HOST", Value: "localhost", Public: true}
	c.Assert(app.Env["DATABASE_HOST"], check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Setting 1 new environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "Envs.0.Name", "value": "DATABASE_HOST"},
			{"name": "Envs.0.Value", "value": "localhost"},
			{"name": "NoRestart", "value": "true"},
			{"name": "Private", "value": ""},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetEnvMissingFormBody(c *check.C) {
	a := app.App{Name: "rock", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "missing form body\n"
	c.Assert(recorder.Body.String(), check.Equals, msg)
}

func (s *S) TestSetEnvHandlerReturnsBadRequestIfVariablesAreMissing(c *check.C) {
	a := app.App{Name: "rock", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/rock/env", strings.NewReader(""))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	msg := "You must provide the list of environment variables\n"
	c.Assert(recorder.Body.String(), check.Equals, msg)
}

func (s *S) TestSetEnvHandlerReturnsNotFoundIfTheAppDoesNotExist(c *check.C) {
	b := strings.NewReader("noRestart=false&private=&false&envs.0.name=DATABASE_HOST&envs.0.value=localhost")
	request, err := http.NewRequest("POST", "/apps/unknown/env", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestSetEnvHandlerReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "rock-and-roll", Platform: "zend"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateEnvSet,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"DATABASE_HOST", "localhost"},
		},
		NoRestart: false,
		Private:   false,
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSetEnvInvalidEnvName(c *check.C) {
	a := app.App{Name: "black-dog", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env", a.Name)
	d := apiTypes.Envs{
		Envs: []struct{ Name, Value string }{
			{"INVALID ENV", "value"},
		},
	}
	v, err := form.EncodeToValues(&d)
	c.Assert(err, check.IsNil)
	b := strings.NewReader(v.Encode())
	request, err := http.NewRequest(http.MethodPost, url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUnsetEnv(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Unsetting 1 environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvNoRestart(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	expected := a.Env
	delete(expected, "DATABASE_HOST")
	url := fmt.Sprintf("/apps/%s/env?noRestart=true&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("swift")
	c.Assert(err, check.IsNil)
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(recorder.Body.String(), check.Equals,
		`{"Message":"---- Unsetting 1 environment variables ----\n"}
`)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": "DATABASE_HOST"},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetEnvHandlerRemovesAllGivenEnvironmentVariables(c *check.C) {
	a := app.App{
		Name:     "let-it-be",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("let-it-be")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{
		"DATABASE_PASSWORD": {
			Name:   "DATABASE_PASSWORD",
			Value:  "secret",
			Public: false,
		},
	}
	c.Assert(app.Env, check.DeepEquals, expected)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.env.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "env", "value": []string{"DATABASE_HOST", "DATABASE_USER"}},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetHandlerRemovesPrivateVariables(c *check.C) {
	a := app.App{
		Name:     "letitbe",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST&env=DATABASE_USER&env=DATABASE_PASSWORD", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	app, err := app.GetByName("letitbe")
	c.Assert(err, check.IsNil)
	expected := map[string]bind.EnvVar{}
	c.Assert(app.Env, check.DeepEquals, expected)
}

func (s *S) TestUnsetEnvVariablesMissing(c *check.C) {
	a := app.App{
		Name:     "swift",
		Platform: "zend",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"DATABASE_HOST":     {Name: "DATABASE_HOST", Value: "localhost", Public: true},
			"DATABASE_USER":     {Name: "DATABASE_USER", Value: "root", Public: true},
			"DATABASE_PASSWORD": {Name: "DATABASE_PASSWORD", Value: "secret", Public: false},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/swift/env?noRestart=false&env=", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide the list of environment variables.\n")
}

func (s *S) TestUnsetEnvAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/env?noRestart=false&env=ble", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "App unknown not found.\n")
}

func (s *S) TestUnsetEnvUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "mountain-mama"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateEnvUnset,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/env?noRestart=false&env=DATABASE_HOST", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAddCName(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=leper.secretcompany.com&cname=blog.tsuru.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"leper.secretcompany.com", "blog.tsuru.com"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.add",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": []interface{}{"leper.secretcompany.com", "blog.tsuru.com"}},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameAcceptsWildCard(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=*.leper.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{"*.leper.secretcompany.com"})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.add",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "*.leper.secretcompany.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameErrsOnInvalidCName(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=_leper.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid cname\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.cname.add",
		ErrorMatches: "Invalid cname",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "_leper.secretcompany.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsEmpty(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/leper/cname", strings.NewReader("cname="))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid cname\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.cname.add",
		ErrorMatches: "Invalid cname",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": ""},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAddCNameHandlerReturnsBadRequestWhenCNameIsMissing(c *check.C) {
	a := app.App{Name: "leper", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	bodies := []io.Reader{nil, strings.NewReader("")}
	for _, b := range bodies {
		request, err := http.NewRequest("POST", "/apps/leper/cname", b)
		c.Assert(err, check.IsNil)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Authorization", "b "+s.token.GetValue())
		recorder := httptest.NewRecorder()
		s.testServer.ServeHTTP(recorder, request)
		c.Check(recorder.Code, check.Equals, http.StatusBadRequest)
		c.Check(recorder.Body.String(), check.Equals, "You must provide the cname.\n")
	}
}

func (s *S) TestAddCNameHandlerUnknownApp(c *check.C) {
	b := strings.NewReader("cname=leper.secretcompany.com")
	request, err := http.NewRequest("POST", "/apps/unknown/cname", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestAddCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := app.App{Name: "lost", Platform: "vougan", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname", a.Name)
	b := strings.NewReader("cname=lost.secretcompany.com")
	request, err := http.NewRequest("POST", url, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateCname,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRemoveCNameHandler(c *check.C) {
	a := app.App{
		Name:      "leper",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("foo.bar.com")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": "foo.bar.com"},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveCNameTwoCnames(c *check.C) {
	a := app.App{
		Name:      "leper",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AddCName("foo.bar.com")
	c.Assert(err, check.IsNil)
	err = a.AddCName("bar.com")
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com&cname=bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	app, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	c.Assert(app.CName, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.cname.remove",
		StartCustomData: []map[string]interface{}{
			{"name": "cname", "value": []interface{}{"foo.bar.com", "bar.com"}},
			{"name": ":app", "value": "leper"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveCNameUnknownApp(c *check.C) {
	request, err := http.NewRequest("DELETE", "/apps/unknown/cname?cname=foo.bar.com", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRemoveCNameHandlerUserWithoutAccessToTheApp(c *check.C) {
	a := app.App{
		Name:     "lost",
		Platform: "vougan",
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateCnameRemove,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/cname?cname=foo.bar.com", a.Name)
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogShouldReturnNotFoundWhenAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/log/?:app=unknown&lines=10", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestAppLogReturnsForbiddenIfTheGivenUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "lost", Platform: "vougan"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, "no-access"),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsMissing(c *check.C) {
	url := "/apps/something/log/?:app=doesntmatter"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, `Parameter "lines" is mandatory.`)
}

func (s *S) TestAppLogReturnsBadRequestIfNumberOfLinesIsNotAnInteger(c *check.C) {
	url := "/apps/something/log/?:app=doesntmatter&lines=2.34"
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, `Parameter "lines" must be an integer.`)
}

func (s *S) TestAppLogFollow(c *check.C) {
	a := app.App{Name: "lost1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	path := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1"
	request, err := http.NewRequest("GET", path, nil)
	c.Assert(err, check.IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	recorder := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logErr := appLog(recorder, request, token)
		c.Assert(logErr, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []app.Applog{}
		logErr = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(logErr, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "x")
	}()
	var listener *app.LogListener
	timeout := time.After(5 * time.Second)
	for listener == nil {
		select {
		case <-timeout:
			c.Fatal("timeout after 5 seconds")
		case <-time.After(50 * time.Millisecond):
		}
		logTracker.Lock()
		for listener = range logTracker.conn {
		}
		logTracker.Unlock()
	}
	err = a.Log("x", "", "")
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()
}

func (s *S) TestAppLogFollowWithFilter(c *check.C) {
	a := app.App{Name: "lost2", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	path := "/apps/something/log/?:app=" + a.Name + "&lines=10&follow=1&source=web"
	request, err := http.NewRequest("GET", path, nil)
	c.Assert(err, check.IsNil)
	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	recorder := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logErr := appLog(recorder, request, token)
		c.Assert(logErr, check.IsNil)
		splitted := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
		c.Assert(splitted, check.HasLen, 2)
		c.Assert(splitted[0], check.Equals, "[]")
		logs := []app.Applog{}
		logErr = json.Unmarshal([]byte(splitted[1]), &logs)
		c.Assert(logErr, check.IsNil)
		c.Assert(logs, check.HasLen, 1)
		c.Assert(logs[0].Message, check.Equals, "y")
	}()
	var listener *app.LogListener
	timeout := time.After(5 * time.Second)
	for listener == nil {
		select {
		case <-timeout:
			c.Fatal("timeout after 5 seconds")
		case <-time.After(50 * time.Millisecond):
		}
		logTracker.Lock()
		for listener = range logTracker.conn {
		}
		logTracker.Unlock()
	}
	err = a.Log("x", "app", "")
	c.Assert(err, check.IsNil)
	err = a.Log("y", "web", "")
	c.Assert(err, check.IsNil)
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()
}

func (s *S) TestAppLogShouldHaveContentType(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
}

func (s *S) TestAppLogSelectByLines(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	for i := 0; i < 15; i++ {
		a.Log(strconv.Itoa(i), "source", "")
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []app.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 10)
}

func (s *S) TestAppLogSelectBySource(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a.Log("mars log", "mars", "")
	a.Log("earth log", "earth", "")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&source=mars&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []app.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "mars log")
	c.Assert(logs[0].Source, check.Equals, "mars")
}

func (s *S) TestAppLogSelectByUnit(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	a.Log("mars log", "mars", "prospero")
	a.Log("earth log", "earth", "caliban")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&unit=caliban&lines=10", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []app.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 1)
	c.Assert(logs[0].Message, check.Equals, "earth log")
	c.Assert(logs[0].Source, check.Equals, "earth")
	c.Assert(logs[0].Unit, check.Equals, "caliban")
}

func (s *S) TestAppLogSelectByLinesShouldReturnTheLastestEntries(c *check.C) {
	a := app.App{Name: "lost", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	now := time.Now()
	coll := s.logConn.Logs(a.Name)
	for i := 0; i < 15; i++ {
		l := app.Applog{
			Date:    now.Add(time.Duration(i) * time.Hour),
			Message: strconv.Itoa(i),
			Source:  "source",
			AppName: a.Name,
		}
		coll.Insert(l)
	}
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=3", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var logs []app.Applog
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 3)
	c.Assert(logs[0].Message, check.Equals, "12")
	c.Assert(logs[1].Message, check.Equals, "13")
	c.Assert(logs[2].Message, check.Equals, "14")
}

func (s *S) TestAppLogShouldReturnLogByApp(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app1.Log("app1 log", "source", "")
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	app2.Log("app2 log", "sourc ", "")
	app3 := app.App{Name: "app3", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app3, s.user)
	c.Assert(err, check.IsNil)
	app3.Log("app3 log", "tsuru", "")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	url := fmt.Sprintf("/apps/%s/log/?:app=%s&lines=10", app3.Name, app3.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request.Header.Set("Content-Type", "application/json")
	err = appLog(recorder, request, token)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	logs := []app.Applog{}
	err = json.Unmarshal(recorder.Body.Bytes(), &logs)
	c.Assert(err, check.IsNil)
	var logged bool
	for _, log := range logs {
		// Should not show the app1 log
		c.Assert(log.Message, check.Not(check.Equals), "app1 log")
		// Should not show the app2 log
		c.Assert(log.Message, check.Not(check.Equals), "app2 log")
		if log.Message == "app3 log" {
			logged = true
		}
	}
	// Should show the app3 log
	c.Assert(logged, check.Equals, true)
}

func (s *S) TestBindHandlerEndpointIsDown(c *check.C) {
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "http://localhost:1234"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	errRegex := `Failed to bind app "painkiller" to service instance "mysql/my-mysql":.*`
	c.Assert(recorder.Body.String(), check.Matches, errRegex+"\n")
	c.Assert(eventtest.EventDesc{
		Target:       appTarget(a.Name),
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.bind",
		ErrorMatches: errRegex,
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestBindHandler(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	s.provisioner.PrepareOutput([]byte("exported"))
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	b := strings.NewReader("noRestart=false")
	request, err := http.NewRequest("PUT", u, b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	c.Assert(allEnvs["DATABASE_USER"], check.DeepEquals, bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false})
	c.Assert(allEnvs["DATABASE_PASSWORD"], check.DeepEquals, bind.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false})
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 8)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Setting 3 new environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"restarting app"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"\nInstance \"my-mysql\" is now bound to the app \"painkiller\".\n"}`)
	c.Assert(parts[3], check.Equals, `{"Message":"The following environment variables are available for use in your app:\n\n"}`)
	c.Assert(parts[4], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[5], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[6], check.Matches, `{"Message":"- TSURU_SERVICES\\n"}`)
	c.Assert(parts[7], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestBindHandlerReturns400IfServiceIsBlacklistedAndItsTheOnlyService(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "demacia", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  s.Pool,
		Field:     pool.ConstraintTypeService,
		Values:    []string{"mysql"},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "pain-gaming", Platform: "zend", TeamOwner: s.team.Name, Env: map[string]bind.EnvVar{}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text/plain; charset=utf-8")
}

func (s *S) TestBindHandlerReturns400IfServiceIsBlacklistedAndMoreServicesAvailable(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "demacia", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	err = pool.SetPoolConstraint(&pool.PoolConstraint{
		PoolExpr:  s.Pool,
		Field:     pool.ConstraintTypeService,
		Values:    []string{"mysql"},
		Blacklist: true,
	})
	c.Assert(err, check.IsNil)
	srvc2 := service.Service{Name: "varus", Endpoint: map[string]string{"production": ts.URL}, Password: "varus123", OwnerTeams: []string{s.team.Name}}
	err = service.Create(srvc2)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "pain-gaming", Platform: "zend", TeamOwner: s.team.Name, Env: map[string]bind.EnvVar{}}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "service \"mysql\" is not available for pool \"test1\". Available services are: \"varus\"\n")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "text/plain; charset=utf-8")
}

func (s *S) TestBindHandlerWithoutEnvsDontRestartTheApp(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{a.Name})
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, check.IsNil)
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 2)
	c.Assert(parts[0], check.Equals, `{"Message":"\nInstance \"my-mysql\" is now bound to the app \"painkiller\".\n"}`)
	c.Assert(parts[1], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
	c.Assert(s.provisioner.Restarts(&a, ""), check.Equals, 0)
}

func (s *S) TestBindHandlerErrorShowsStatusMessage(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance.ServiceName, instance.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "false")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "Failed to bind the instance \"mysql/my-mysql\" to the app \"painkiller\": invalid response:  (code: 500) (\"my-mysql\" is down)\n")
}

func (s *S) TestBindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := app.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/unknown/instances/unknown/%s?:instance=unknown&:app=%s&:service=unknown&noRestart=false", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, a.Name, instance.Name, a.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestBindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/unknown?:instance=%s&:app=unknown&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, instance.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestBindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateBind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "serviceapp", Platform: "zend"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=false", instance.ServiceName,
		instance.Name, a.Name, instance.Name, a.Name, instance.ServiceName)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = bindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestBindWithManyInstanceNameWithSameNameAndNoRestartFlag(c *check.C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"DATABASE_USER":"root","DATABASE_PASSWORD":"s3cr3t"}`))
	}))
	defer ts.Close()
	srvcs := []service.Service{
		{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, srvc := range srvcs {
		err := service.Create(srvc)
		c.Assert(err, check.IsNil)
	}
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
	}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	instance2 := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql2",
		Teams:       []string{s.team.Name},
	}
	err = s.conn.ServiceInstances().Insert(instance2)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
		Env:       map[string]bind.EnvVar{},
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnits(&a, 1, "web", nil)
	u := fmt.Sprintf("/services/%s/instances/%s/%s", instance2.ServiceName, instance2.Name, a.Name)
	v := url.Values{}
	v.Set("noRestart", "true")
	request, err := http.NewRequest("PUT", u, strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.provisioner.PrepareOutput([]byte("exported"))
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	var result service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance2.Name, "service_name": instance2.ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{a.Name})
	err = s.conn.Apps().Find(bson.M{"name": a.Name}).One(&a)
	c.Assert(err, check.IsNil)
	allEnvs := a.Envs()
	c.Assert(allEnvs["DATABASE_USER"], check.DeepEquals, bind.EnvVar{Name: "DATABASE_USER", Value: "root", Public: false})
	c.Assert(allEnvs["DATABASE_PASSWORD"], check.DeepEquals, bind.EnvVar{Name: "DATABASE_PASSWORD", Value: "s3cr3t", Public: false})
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 7)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Setting 3 new environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"\nInstance \"my-mysql\" is now bound to the app \"painkiller\".\n"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"The following environment variables are available for use in your app:\n\n"}`)
	c.Assert(parts[3], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[4], check.Matches, `{"Message":"- DATABASE_(USER|PASSWORD)\\n"}`)
	c.Assert(parts[5], check.Matches, `{"Message":"- TSURU_SERVICES\\n"}`)
	c.Assert(parts[6], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name, "service_name": instance.ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.bind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance2.Name},
			{"name": ":service", "value": instance2.ServiceName},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindHandler(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bind.ServiceEnvVar{
		EnvVar: bind.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := otherApp.Envs()
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for <-t; atomic.LoadInt32(&called) == 0; <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Error("Failed to call API after 1 second.")
	}
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 4)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Unsetting 1 environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"restarting app"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"\nInstance \"my-mysql\" is not bound to the app \"painkiller\" anymore.\n"}`)
	c.Assert(parts[3], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindNoRestartFlag(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bind.ServiceEnvVar{
		EnvVar: bind.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := otherApp.Envs()
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	ch := make(chan bool)
	go func() {
		t := time.Tick(1)
		for <-t; atomic.LoadInt32(&called) == 0; <-t {
		}
		ch <- true
	}()
	select {
	case <-ch:
		c.Succeed()
	case <-time.After(1e9):
		c.Error("Failed to call API after 1 second.")
	}
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 3)
	c.Assert(parts[0], check.Equals, `{"Message":"---- Unsetting 1 environment variables ----\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"\nInstance \"my-mysql\" is not bound to the app \"painkiller\" anymore.\n"}`)
	c.Assert(parts[2], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "noRestart", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindForceFlag(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind-app" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("my unbind err"))
		}
	}))
	defer ts.Close()
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	otherApp, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	otherApp.ServiceEnvs = append(otherApp.ServiceEnvs, bind.ServiceEnvVar{
		EnvVar: bind.EnvVar{
			Name:  "DATABASE_HOST",
			Value: "arrea",
		},
		InstanceName: instance.Name,
		ServiceName:  instance.ServiceName,
	})
	otherApp.Env["MY_VAR"] = bind.EnvVar{Name: "MY_VAR", Value: "123"}
	err = s.conn.Apps().Update(bson.M{"name": otherApp.Name}, otherApp)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&force=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermServiceUpdate,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	err = unbindServiceInstance(recorder, req, token)
	c.Assert(err, check.IsNil)
	err = s.conn.ServiceInstances().Find(bson.M{"name": instance.Name}).One(&instance)
	c.Assert(err, check.IsNil)
	c.Assert(instance.Apps, check.DeepEquals, []string{})
	otherApp, err = app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	expected := bind.EnvVar{
		Name:  "MY_VAR",
		Value: "123",
	}
	allEnvs := otherApp.Envs()
	c.Assert(allEnvs["MY_VAR"], check.DeepEquals, expected)
	_, ok := allEnvs["DATABASE_HOST"]
	c.Assert(ok, check.Equals, false)
	parts := strings.Split(recorder.Body.String(), "\n")
	c.Assert(parts, check.HasLen, 5)
	c.Assert(parts[0], check.Equals, `{"Message":"[unbind-app-endpoint] ignored error due to force: Failed to unbind (\"/resources/my-mysql/bind-app\"): invalid response: my unbind err (code: 500)\n"}`)
	c.Assert(parts[1], check.Equals, `{"Message":"---- Unsetting 1 environment variables ----\n"}`)
	c.Assert(parts[2], check.Equals, `{"Message":"restarting app"}`)
	c.Assert(parts[3], check.Equals, `{"Message":"\nInstance \"my-mysql\" is not bound to the app \"painkiller\" anymore.\n"}`)
	c.Assert(parts[4], check.Equals, "")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  token.GetUserName(),
		Kind:   "app.update.unbind",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": ":instance", "value": instance.Name},
			{"name": ":service", "value": instance.ServiceName},
			{"name": "force", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnbindForceFlagUnauthorized(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	srvc := service.Service{Name: "mysql", Endpoint: map[string]string{"production": "myendpoint"}, Password: "abcde", OwnerTeams: []string{s.team.Name}}
	err := service.Create(srvc)
	c.Assert(err, check.IsNil)
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instance := service.ServiceInstance{
		Name:        "my-mysql",
		ServiceName: "mysql",
		Teams:       []string{s.team.Name},
		Apps:        []string{"painkiller"},
		BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
	}
	err = s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&force=true", instance.ServiceName, instance.Name, a.Name,
		instance.ServiceName, instance.Name, a.Name)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	err = unbindServiceInstance(recorder, req, token)
	c.Assert(err, check.Equals, permission.ErrUnauthorized)
}

func (s *S) TestUnbindWithSameInstanceName(c *check.C) {
	s.provisioner.PrepareOutput([]byte("exported"))
	var called int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/resources/my-mysql/bind" {
			atomic.StoreInt32(&called, 1)
		}
	}))
	defer ts.Close()
	srvcs := []service.Service{
		{Name: "mysql", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
		{Name: "mysql2", Endpoint: map[string]string{"production": ts.URL}, Password: "abcde", OwnerTeams: []string{s.team.Name}},
	}
	for _, srvc := range srvcs {
		err := service.Create(srvc)
		c.Assert(err, check.IsNil)
	}
	a := app.App{
		Name:      "painkiller",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := s.provisioner.Units(&a)
	c.Assert(err, check.IsNil)
	instances := []service.ServiceInstance{
		{
			Name:        "my-mysql",
			ServiceName: "mysql",
			Teams:       []string{s.team.Name},
			Apps:        []string{"painkiller"},
			BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
		},
		{
			Name:        "my-mysql",
			ServiceName: "mysql2",
			Teams:       []string{s.team.Name},
			Apps:        []string{"painkiller"},
			BoundUnits:  []service.Unit{{ID: units[0].ID, IP: units[0].IP}},
		},
	}
	for _, instance := range instances {
		err = s.conn.ServiceInstances().Insert(instance)
		c.Assert(err, check.IsNil)
	}
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:instance=%s&:app=%s&:service=%s&noRestart=true", instances[1].ServiceName, instances[1].Name, a.Name,
		instances[1].Name, a.Name, instances[1].ServiceName)
	req, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, req, s.token)
	c.Assert(err, check.IsNil)
	var result service.ServiceInstance
	err = s.conn.ServiceInstances().Find(bson.M{"name": instances[1].Name, "service_name": instances[1].ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{})
	err = s.conn.ServiceInstances().Find(bson.M{"name": instances[0].Name, "service_name": instances[0].ServiceName}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Apps, check.DeepEquals, []string{a.Name})
}

func (s *S) TestUnbindHandlerReturns404IfTheInstanceDoesNotExist(c *check.C) {
	a := app.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/instances/unknown/%s?:instance=unknown&:app=%s&noRestart=false", a.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e.Message, check.Equals, service.ErrServiceInstanceNotFound.Error())
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheInstance(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql"}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "serviceapp", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestUnbindHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/unknown?:service=%s&:instance=%s&:app=unknown&noRestart=false", instance.ServiceName,
		instance.Name, instance.ServiceName, instance.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
	c.Assert(e, check.ErrorMatches, "^App unknown not found.$")
}

func (s *S) TestUnbindHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermServiceInstanceUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	}, permission.Permission{
		Scheme:  permission.PermAppUpdateUnbind,
		Context: permission.Context(permTypes.CtxTeam, "other-team"),
	})
	instance := service.ServiceInstance{Name: "my-mysql", ServiceName: "mysql", Teams: []string{s.team.Name}}
	err := s.conn.ServiceInstances().Insert(instance)
	c.Assert(err, check.IsNil)
	a := app.App{Name: "serviceapp", Platform: "zend"}
	err = s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/services/%s/instances/%s/%s?:service=%s&:instance=%s&:app=%s&noRestart=false", instance.ServiceName, instance.Name,
		a.Name, instance.ServiceName, instance.Name, a.Name)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = unbindServiceInstance(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestRestartHandler(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := app.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/restart", a.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"---- Restarting the app \\\"stress\\\" ----\\n\"}\n{\"Message\":\"restarting app\"}\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.restart",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRestartHandlerSingleProcess(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := app.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/restart", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"---- Restarting process \\\"web\\\" ----\\n\"}\n{\"Message\":\"restarting app\"}\n")
	restarts := s.provisioner.Restarts(&a, "web")
	c.Assert(restarts, check.Equals, 1)
	restarts = s.provisioner.Restarts(&a, "worker")
	c.Assert(restarts, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.restart",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRestartHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/unknown/restart?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestRestartHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "nightmist"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateRestart,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/restart?:app=%s", a.Name, a.Name)
	request, err := http.NewRequest("GET", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = restart(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestSleepHandler(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := app.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/sleep", a.Name)
	body := strings.NewReader("proxy=http://example.com")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"\\n ---\\u003e Putting the app \\\"stress\\\" to sleep\\n\"}\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.sleep",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "proxy", "value": "http://example.com"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSleepHandlerReturns400IfTheProxyIsNotSet(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := app.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("POST", "/apps/stress/sleep?:app=stress", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = sleep(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusBadRequest)
	c.Assert(e.Message, check.Equals, "Empty proxy URL")
}

func (s *S) TestSleepHandlerReturns404IfTheAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("POST", "/apps/unknown/sleep?:app=unknown", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = sleep(recorder, request, s.token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusNotFound)
}

func (s *S) TestSleepHandlerReturns403IfTheUserDoesNotHaveAccessToTheApp(c *check.C) {
	a := app.App{Name: "nightmist"}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateSleep,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	url := fmt.Sprintf("/apps/%s/sleep?:app=%s&proxy=http://example.com", a.Name, a.Name)
	request, err := http.NewRequest("POST", url, nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	err = sleep(recorder, request, token)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.HTTP)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Code, check.Equals, http.StatusForbidden)
}

type LogList []app.Applog

func (l LogList) Len() int           { return len(l) }
func (l LogList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l LogList) Less(i, j int) bool { return l[i].Message < l[j].Message }

func (s *S) TestAddLog(c *check.C) {
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Add("message", "message 1")
	v.Add("message", "message 2")
	v.Add("message", "message 3")
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdateLog,
		Context: permission.Context(permTypes.CtxTeam, s.team.Name),
	})
	request, err := http.NewRequest("POST", "/apps/myapp/log", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	v = url.Values{}
	v.Add("message", "message 4")
	v.Add("message", "message 5")
	v.Set("source", "mysource")
	withSourceRequest, err := http.NewRequest("POST", "/apps/myapp/log", strings.NewReader(v.Encode()))
	c.Assert(err, check.IsNil)
	withSourceRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSourceRequest.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, withSourceRequest)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	want := []string{
		"message 1",
		"message 2",
		"message 3",
		"message 4",
		"message 5",
	}
	wantSource := []string{
		"app",
		"app",
		"app",
		"mysource",
		"mysource",
	}
	logs, err := a.LastLogs(5, app.Applog{})
	c.Assert(err, check.IsNil)
	got := make([]string, len(logs))
	gotSource := make([]string, len(logs))
	sort.Sort(LogList(logs))
	for i, l := range logs {
		got[i] = l.Message
		gotSource[i] = l.Source
	}
	c.Assert(got, check.DeepEquals, want)
	c.Assert(gotSource, check.DeepEquals, wantSource)
}

func (s *S) TestGetApp(c *check.C) {
	a := app.App{Name: "testapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	expected, err := app.GetByName(a.Name)
	c.Assert(err, check.IsNil)
	r, err := http.NewRequest(http.MethodGet, "", nil)
	c.Assert(err, check.IsNil)
	app, err := getAppFromContext(a.Name, r)
	c.Assert(err, check.IsNil)
	c.Assert(app, check.DeepEquals, *expected)
}

func (s *S) TestSwap(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": app1.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, appTypes.AppLock{})
	err = s.conn.Apps().Find(bson.M{"name": app2.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, appTypes.AppLock{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app1.Name),
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: app2.Name}, Lock: true},
		},
		Owner: s.token.GetUserName(),
		Kind:  "app.update.swap",
		StartCustomData: []map[string]interface{}{
			{"name": "app1", "value": app1.Name},
			{"name": "app2", "value": app2.Name},
			{"name": "cnameOnly", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSwapCnameOnly(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=true")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": app1.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, appTypes.AppLock{})
	err = s.conn.Apps().Find(bson.M{"name": app2.Name}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock, check.Equals, appTypes.AppLock{})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app1.Name),
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: app2.Name}, Lock: true},
		},
		Owner: s.token.GetUserName(),
		Kind:  "app.update.swap",
		StartCustomData: []map[string]interface{}{
			{"name": "app1", "value": app1.Name},
			{"name": "app2", "value": app2.Name},
			{"name": "cnameOnly", "value": "true"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSwapApp1Locked(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name, Lock: appTypes.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Matches, "app1: App locked by x, running /test. Acquired in .*\n")
}

func (s *S) TestSwapApp2Locked(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "zend", TeamOwner: s.team.Name, Lock: appTypes.AppLock{
		Locked: true, Reason: "/test", Owner: "x",
	}}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Matches, "app2: App locked by x, running /test. Acquired in .*\n")
}

func (s *S) TestSwapIncompatiblePlatforms(c *check.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "y"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&app2)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(recorder.Body.String(), check.Equals, "platforms don't match\n")
	c.Assert(eventtest.EventDesc{
		Target: appTarget(app1.Name),
		ExtraTargets: []event.ExtraTarget{
			{Target: event.Target{Type: "app", Value: app2.Name}, Lock: true},
		},
		Owner:        s.token.GetUserName(),
		Kind:         "app.update.swap",
		ErrorMatches: "platforms don't match",
		StartCustomData: []map[string]interface{}{
			{"name": "app1", "value": app1.Name},
			{"name": "app2", "value": app2.Name},
			{"name": "cnameOnly", "value": "false"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSwapIncompatibleUnits(c *check.C) {
	app1 := app.App{Name: "app1", Teams: []string{s.team.Name}, Platform: "x"}
	err := s.conn.Apps().Insert(&app1)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&app1)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Teams: []string{s.team.Name}, Platform: "x"}
	err = s.conn.Apps().Insert(&app2)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&app2)
	c.Assert(err, check.IsNil)
	s.provisioner.AddUnit(&app2, provision.Unit{})
	b := strings.NewReader("app1=app1&app2=app2&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusPreconditionFailed)
	c.Assert(recorder.Body.String(), check.Equals, "number of units doesn't match\n")
}

func (s *S) TestSwapIncompatibleAppsForceSwap(c *check.C) {
	app1 := app.App{Name: "app1", Platform: "x", TeamOwner: s.team.Name}
	err := app.CreateApp(&app1, s.user)
	c.Assert(err, check.IsNil)
	app2 := app.App{Name: "app2", Platform: "y", TeamOwner: s.team.Name}
	err = app.CreateApp(&app2, s.user)
	c.Assert(err, check.IsNil)
	b := strings.NewReader("app1=app1&app2=app2&force=true&cnameOnly=false")
	request, err := http.NewRequest("POST", "/swap", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
}

func (s *S) TestStartHandler(c *check.C) {
	config.Set("docker:router", "fake")
	defer config.Unset("docker:router")
	a := app.App{
		Name:      "stress",
		Platform:  "zend",
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/start", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"\\n ---\\u003e Starting the process \\\"web\\\"\\n\"}\n")
	starts := s.provisioner.Starts(&a, "web")
	c.Assert(starts, check.Equals, 1)
	starts = s.provisioner.Starts(&a, "worker")
	c.Assert(starts, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.start",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestStopHandler(c *check.C) {
	a := app.App{Name: "stress", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	url := fmt.Sprintf("/apps/%s/stop", a.Name)
	body := strings.NewReader("process=web")
	request, err := http.NewRequest("POST", url, body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "b "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	c.Assert(recorder.Body.String(), check.Equals, "{\"Message\":\"\\n ---\\u003e Stopping the process \\\"web\\\"\\n\"}\n")
	stops := s.provisioner.Stops(&a, "web")
	c.Assert(stops, check.Equals, 1)
	stops = s.provisioner.Stops(&a, "worker")
	c.Assert(stops, check.Equals, 0)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.stop",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "web"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestForceDeleteLock(c *check.C) {
	a := app.App{Name: "locked", Lock: appTypes.AppLock{Locked: true}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock.Locked, check.Equals, false)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.unlock",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestForceDeleteLockOnlyWithPermission(c *check.C) {
	a := app.App{Name: "locked", Lock: appTypes.AppLock{Locked: true}, Teams: []string{s.team.Name}}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/apps/locked/lock", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
	var dbApp app.App
	err = s.conn.Apps().Find(bson.M{"name": "locked"}).One(&dbApp)
	c.Assert(err, check.IsNil)
	c.Assert(dbApp.Lock.Locked, check.Equals, true)
}

func (s *S) TestRegisterUnit(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	oldIP := units[0].IP
	body := strings.NewReader("hostname=" + units[0].ID)
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("X-Agent-Version", "0.2.4")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	envMap := map[interface{}]interface{}{}
	for _, envVar := range result {
		envMap[envVar["name"]] = envVar["value"]
	}
	c.Assert(envMap["MY_VAR_1"], check.Equals, "value1")
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].IP, check.Equals, oldIP+"-updated")
}

func (s *S) TestRegisterUnitInvalidUnit(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("hostname=invalid-unit-host")
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("X-Agent-Version", "0.2.4")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "unit \"invalid-unit-host\" not found\n")
}

func (s *S) TestRegisterUnitOutdatedDeployAgent(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Teams:    []string{s.team.Name},
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
	}
	err := s.conn.Apps().Insert(a)
	c.Assert(err, check.IsNil)
	err = s.provisioner.Provision(&a)
	c.Assert(err, check.IsNil)
	body := strings.NewReader("hostname=invalid-unit-host")
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("User-Agent", "Go-http-client/1.1")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Please contact admin. python platform is using outdated deploy-agent version, minimum required version is 0.2.4\n")

	body = strings.NewReader("hostname=invalid-unit-host")
	request, err = http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("User-Agent", "tsuru-deploy-agent/1.1")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder = httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Please contact admin. python platform is using outdated deploy-agent version, minimum required version is 0.2.4\n")
}

func (s *S) TestRegisterUnitOtherUA(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	body := strings.NewReader("hostname=" + units[0].ID)
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("User-Agent", "curl/1.1")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestRegisterUnitWithCustomData(c *check.C) {
	a := app.App{
		Name:     "myappx",
		Platform: "python",
		Env: map[string]bind.EnvVar{
			"MY_VAR_1": {Name: "MY_VAR_1", Value: "value1", Public: true},
		},
		TeamOwner: s.team.Name,
	}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = s.provisioner.AddUnits(&a, 1, "web", nil)
	c.Assert(err, check.IsNil)
	units, err := a.Units()
	c.Assert(err, check.IsNil)
	oldIP := units[0].IP
	v := url.Values{}
	v.Set("hostname", units[0].ID)
	v.Set("customdata", `{"mydata": "something"}`)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/apps/myappx/units/register", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("X-Agent-Version", "0.2.4")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	result := []map[string]interface{}{}
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	envMap := map[interface{}]interface{}{}
	for _, envVar := range result {
		envMap[envVar["name"]] = envVar["value"]
	}
	c.Assert(envMap["MY_VAR_1"], check.Equals, "value1")
	units, err = a.Units()
	c.Assert(err, check.IsNil)
	c.Assert(units[0].IP, check.Equals, oldIP+"-updated")
	c.Assert(s.provisioner.CustomData(&a), check.DeepEquals, map[string]interface{}{
		"mydata": "something",
	})
}

func (s *S) TestMetricEnvs(c *check.C) {
	err := nodecontainer.AddNewContainer("", &nodecontainer.NodeContainerConfig{
		Name: nodecontainer.BsDefaultName,
		Config: docker.Config{
			Image: "img1",
			Env: []string{
				"OTHER_ENV=asd",
				"METRICS_BACKEND=fake",
			},
		},
	})
	c.Assert(err, check.IsNil)
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name}
	err = app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/myappx/metric/envs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "{\"METRICS_BACKEND\":\"fake\"}\n")
}

func (s *S) TestMetricEnvsWhenUserDoesNotHaveAccess(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend"}
	err := s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppReadMetric,
		Context: permission.Context(permTypes.CtxApp, "-invalid-"),
	})
	request, err := http.NewRequest("GET", "/apps/myappx/metric/envs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusForbidden)
}

func (s *S) TestMetricEnvsWhenAppDoesNotExist(c *check.C) {
	request, err := http.NewRequest("GET", "/apps/myappx/metric/envs", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Matches, "^App .* not found.\n$")
}

func (s *S) TestRebuildRoutes(c *check.C) {
	a := app.App{Name: "myappx", Platform: "zend", TeamOwner: s.team.Name, Router: "fake"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	s.provisioner.Provision(&a)
	err = routertest.FakeRouter.AddRoutes(a.Name, []*url.URL{
		{Host: "h1"},
	})
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("dry", "true")
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/apps/myappx/routes", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var parsed map[string]rebuild.RebuildRoutesResult
	json.Unmarshal(recorder.Body.Bytes(), &parsed)
	c.Assert(parsed, check.DeepEquals, map[string]rebuild.RebuildRoutesResult{
		"fake": {
			Removed: []string{"http://h1"},
		},
	})
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.admin.routes",
		StartCustomData: []map[string]interface{}{
			{"name": "dry", "value": "true"},
			{"name": ":app", "value": a.Name},
		},
		EndCustomData: map[string]interface{}{
			"fake.added":   []string(nil),
			"fake.removed": []string{"http://h1"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetCertificate(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.certificate.set",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
			{"name": "certificate", "value": testCert},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestSetCertificateInvalidCname(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app2.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "invalid name\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetCertificateInvalidCertificate(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"myapp.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "myapp.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "x509: certificate is valid for app.io, not myapp.io\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestSetCertificateNonSupportedRouter(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	v := url.Values{}
	v.Set("cname", "app.io")
	v.Set("certificate", testCert)
	v.Set("key", testKey)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("PUT", "/apps/myapp/certificate", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Body.String(), check.Equals, "no router with tls support\n")
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
}

func (s *S) TestUnsetCertificate(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate?cname=app.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(eventtest.EventDesc{
		Target: appTarget(a.Name),
		Owner:  s.token.GetUserName(),
		Kind:   "app.update.certificate.unset",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "cname", "value": "app.io"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestUnsetCertificateWithoutCName(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "You must provide a cname.\n")
}

func (s *S) TestUnsetCertificateInvalidCName(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/apps/myapp/certificate?cname=myapp.io", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "invalid name\n")
}

func (s *S) TestListCertificates(c *check.C) {
	a := app.App{Name: "myapp", TeamOwner: s.team.Name, CName: []string{"app.io"}, Router: "fake-tls"}
	err := app.CreateApp(&a, s.user)
	c.Assert(err, check.IsNil)
	err = a.SetCertificate("app.io", testCert, testKey)
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/apps/myapp/certificate", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var certs map[string]map[string]string
	err = json.Unmarshal(recorder.Body.Bytes(), &certs)
	c.Assert(err, check.IsNil)
	c.Assert(certs, check.DeepEquals, map[string]map[string]string{
		"fake-tls": {
			"app.io":                  testCert,
			"myapp.faketlsrouter.com": "",
		},
	})
}
