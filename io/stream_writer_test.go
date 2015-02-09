// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/check.v1"
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
		return ErrInvalidStreamChunk
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%s-%s\n", e.Source, e.Message)
	}
	return nil
}

func (s *S) TestStreamWriterUsesFormatter(c *check.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	w.Write(data)
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterChukedWrite(c *check.C) {
	entries := []entry{
		{Message: "\nSome\nthing\nhappened\n", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	data, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	l := len(data)
	var buf bytes.Buffer
	w := NewStreamWriter(&buf, testFormatter{})
	_, err = w.Write(data[:l/4])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/4 : l/2])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/2 : l/4*3])
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "")
	_, err = w.Write(data[l/4*3:])
	c.Assert(err, check.IsNil)
	expected := "tsuru-\nSome\nthing\nhappened\n\ntsuru-Something happened again\n"
	c.Assert(buf.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriter(c *check.C) {
	entries := []entry{
		{Message: "Something happened", Source: "tsuru"},
		{Message: "Something happened again", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(b)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(b))
	expected := "tsuru-Something happened\ntsuru-Something happened again\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterMultipleChunksOneMessage(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	n, err := w.Write(append(b, append([]byte("\n"), b...)...))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2*len(b)+1)
	expected := "tsuru-Something 1\ntsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte{})
}

func (s *S) TestStreamWriterInvalidDataNotRead(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInChunk(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\n")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data\n"))
}

func (s *S) TestStreamWriterOnlyInvalidMessage(c *check.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----")
	n, err := w.Write(b)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 5)
	c.Assert(writer.String(), check.Equals, "")
	c.Assert(w.Remaining(), check.DeepEquals, []byte("-----"))
}

func (s *S) TestStreamWriterOnlyInvalidMessageInChunk(c *check.C) {
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	b := []byte("-----\n")
	n, err := w.Write(b)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"-----\\n\"")
	c.Assert(n, check.Equals, 6)
	c.Assert(writer.String(), check.Equals, "")
	c.Assert(w.Remaining(), check.DeepEquals, []byte("-----\n"))
}

func (s *S) TestStreamWriterInvalidDataNotReadInMultipleChunks(c *check.C) {
	entries := []entry{
		{Message: "Something 1", Source: "tsuru"},
	}
	b, err := json.Marshal(entries)
	c.Assert(err, check.IsNil)
	var writer bytes.Buffer
	w := NewStreamWriter(&writer, testFormatter{})
	toWrite := append(b, []byte("\ninvalid data\nmoreinvalid\nsomething")...)
	n, err := w.Write(toWrite)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Unparseable chunk: \"invalid data\\n\"")
	c.Assert(n, check.Equals, len(toWrite))
	expected := "tsuru-Something 1\n"
	c.Assert(writer.String(), check.Equals, expected)
	c.Assert(w.Remaining(), check.DeepEquals, []byte("invalid data\nmoreinvalid\nsomething"))
}

func (s *S) TestSimpleJsonMessageFormatter(c *check.C) {
	formatter := SimpleJsonMessageFormatter{}
	buf := bytes.Buffer{}
	err := formatter.Format(&buf, []byte(`{"message": "mymsg"}`))
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, "mymsg")
	buf = bytes.Buffer{}
	err = formatter.Format(&buf, []byte(`{"message": "mym`))
	c.Assert(err, check.Equals, ErrInvalidStreamChunk)
	c.Assert(buf.String(), check.Equals, "")
	buf = bytes.Buffer{}
	err = formatter.Format(&buf, []byte(`{"message": "mymsg", "error": "myerror"}`))
	c.Assert(err, check.ErrorMatches, "myerror")
	c.Assert(buf.String(), check.Equals, "")
}

func (s *S) TestSimpleJsonMessageEncoderWriter(c *check.C) {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	writer := SimpleJsonMessageEncoderWriter{encoder}
	written, err := writer.Write([]byte("my cool msg"))
	c.Assert(written, check.Equals, 11)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, `{"Message":"my cool msg"}`+"\n")
}
