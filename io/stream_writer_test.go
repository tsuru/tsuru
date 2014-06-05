// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"launchpad.net/gocheck"
)

type entry struct {
	Message string
	Source  string
}

type testFormatter struct{}

func (testFormatter) Format(out io.Writer, data []byte) error {
	var entries []entry
	err := json.Unmarshal(data, &entries)
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%s-%s\n", e.Source, e.Message)
	}
	return nil
}

func (s *S) TestStreamWriterUsesFormatter(c *gocheck.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	w.Write(data)
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterChukedWrite(c *gocheck.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	l := len(data)
	var buf bytes.Buffer
	w := NewStreamWriter(&buf, testFormatter{})
	_, err = w.Write(data[:l/4])
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
	_, err = w.Write(data[l/4 : l/2])
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
	_, err = w.Write(data[l/2 : l/4*3])
	c.Assert(err, gocheck.IsNil)
	c.Assert(buf.String(), gocheck.Equals, "")
	_, err = w.Write(data[l/4*3:])
	c.Assert(err, gocheck.IsNil)
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(buf.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte{})
}

func (s *S) TestStreamWriter(c *gocheck.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(b))
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterMultipleChunksOneMessage(c *gocheck.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(append(b, append([]byte("\n"), b...)...))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 2*len(b)+1)
	expected := "tsuru-Something 1\ntsuru-Something 1\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterInvalidDataNotRead(c *gocheck.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data")...)
	n, err := w.Write(toWrite)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte("invalid data"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInChunk(c *gocheck.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\n")...)
	n, err := w.Write(toWrite)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, gocheck.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte("invalid data\n"))
}

func (s *S) TestStreamWriterOnlyInvalidMessage(c *gocheck.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----")
	n, err := w.Write(b)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 5)
	c.Assert(writer.String(), gocheck.Equals, "")
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte("-----"))
}

func (s *S) TestStreamWriterOnlyInvalidMessageInChunk(c *gocheck.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----\n")
	n, err := w.Write(b)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unparseable chunk: \"-----\\n\"")
	c.Assert(n, gocheck.Equals, 6)
	c.Assert(writer.String(), gocheck.Equals, "")
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte("-----\n"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInMultipleChunks(c *gocheck.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, gocheck.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\nmoreinvalid\nsomething")...)
	n, err := w.Write(toWrite)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, gocheck.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), gocheck.Equals, expected)
	c.Assert(w.Remaining(), gocheck.DeepEquals, []byte("invalid data\nmoreinvalid\nsomething"))
}
