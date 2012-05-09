package cmd

import (
	"io"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func (s *S) TestShouldReturnBodyMessageOnError(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "You must be authenticated to execute this command.")
	}))
	defer ts.Close()

	request, err := http.NewRequest("GET", ts.URL, nil)
	c.Assert(err, IsNil)

	client := NewClient()
	response, err := client.Do(request)
	c.Assert(response, IsNil)
	c.Assert(err.Error(), Equals, "You must be authenticated to execute this command.")
}
