// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	. "launchpad.net/gocheck"
	"net/http/httptest"
)

func (s *S) TestFilteredWriter(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(recorder.Body.Bytes(), DeepEquals, data)
}

func (s *S) TestFilteredWriterShouldReturnsTheDataSize(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(data))
}

func (s *S) TestFilteredWriterHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), Equals, "application/xml")
}

func (s *S) TestFilteredWriterWriteHeader(c *C) {
	recorder := httptest.NewRecorder()
	writer := FilteredWriter{recorder}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, Equals, expectedCode)
}
