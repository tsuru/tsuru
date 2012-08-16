package app

import (
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) mockServer(b string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/tokens" {
			w.Write([]byte(`{"access": {"token": {"id": "token-id-987"}}}`))
		} else {
			w.Write([]byte(b))
		}
	}))
	authUrl = ts.URL
	return ts
}

func (s *S) TestNewTenantSavesInDb(c *C) {
	ts := s.mockServer(`{"tenant": {"id": "uuid123", "name": "tenant name", "description": "tenant desc"}}`)
	defer ts.Close()
	a := App{Name: "myapp"}
	err := db.Session.Apps().Insert(a)
	c.Assert(err, IsNil)
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	t, err := NewTenant(&a)
	c.Assert(err, IsNil)
	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.KeystoneEnv.TenantId, DeepEquals, t)
}
