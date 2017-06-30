// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multipartzip

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	tmpdir string
}

var _ = check.Suite(&S{})

func (s *S) TestCopyZipFile(c *check.C) {
	tempDir, err := ioutil.TempDir("", "TestCopyZipFileDir")
	defer func() {
		os.RemoveAll(tempDir)
	}()
	c.Assert(err, check.IsNil)
	var files = []File{
		{"doge.txt", "Much doge"},
		{"much.txt", "Much mucho"},
		{"WOW/WOW.WOW1", "WOW\nWOW"},
		{"WOW/WOW.WOW2", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW3", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW4", "WOW\nWOW"},
	}
	buf, err := CreateZipBuffer(files)
	c.Assert(err, check.IsNil)
	c.Assert(buf, check.NotNil)
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	for _, f := range r.File {
		err = CopyZipFile(f, tempDir, f.Name)
		c.Assert(err, check.IsNil)
		fstat, errStat := os.Stat(path.Join(tempDir, f.Name))
		c.Assert(errStat, check.IsNil)
		c.Assert(fstat.IsDir(), check.Equals, false)
	}
}

func (s *S) TestCopyZipFileOverAgain(c *check.C) {
	tempDir, err := ioutil.TempDir("", "TestCopyZipFileDir")
	defer func() {
		os.RemoveAll(tempDir)
	}()
	c.Assert(err, check.IsNil)
	// file 1
	var files1 = []File{
		{"doge.txt", "Much doge"},
	}
	buf, err := CreateZipBuffer(files1)
	c.Assert(err, check.IsNil)
	c.Assert(buf, check.NotNil)
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	err = CopyZipFile(r.File[0], tempDir, "doge.txt")
	c.Assert(err, check.IsNil)
	fstat, errStat := os.Stat(path.Join(tempDir, "doge.txt"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	body, err := ioutil.ReadFile(path.Join(tempDir, "doge.txt"))
	c.Assert(err, check.IsNil)
	c.Assert(string(body), check.Equals, "Much doge")
	// file 2
	var files2 = []File{
		{"doge.txt", "Many"},
	}
	buf, err = CreateZipBuffer(files2)
	c.Assert(err, check.IsNil)
	c.Assert(buf, check.NotNil)
	r, err = zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	err = CopyZipFile(r.File[0], tempDir, "doge.txt")
	c.Assert(err, check.IsNil)
	fstat, errStat = os.Stat(path.Join(tempDir, "doge.txt"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	body, err = ioutil.ReadFile(path.Join(tempDir, "doge.txt"))
	c.Assert(err, check.IsNil)
	c.Assert(string(body), check.Equals, "Many")
}

func (s *S) TestExtractZip(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	var files = []File{
		{"doge.txt", "Much doge"},
		{"much.txt", "Much mucho"},
		{"WOW/WOW.WOW1", "WOW\nWOW"},
		{"WOW/WOW.WOW2", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW3", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW4", "WOW\nWOW"},
	}
	buf, err := CreateZipBuffer(files)
	c.Assert(err, check.IsNil)
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "zipfile", "scaffold.zip", boundary, writer, buf)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	formfile := form.File["zipfile"][0]
	tempDir, err := ioutil.TempDir("", "TestCopyZipFileDir")
	defer func() {
		os.RemoveAll(tempDir)
	}()
	c.Assert(err, check.IsNil)
	ExtractZip(formfile, tempDir)
	for _, file := range files {
		body, err := ioutil.ReadFile(path.Join(tempDir, file.Name))
		c.Assert(err, check.IsNil)
		c.Assert(string(body), check.Equals, file.Body)
	}
}

func (s *S) TestValueField(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{
		"committername":  "Barking Doge",
		"committerEmail": "bark@much.com",
		"authorName":     "Doge Dog",
		"authorEmail":    "doge@much.com",
		"message":        "Repository scaffold",
		"branch":         "master",
	}
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "", "", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	value, err := ValueField(form, "branch")
	c.Assert(err, check.IsNil)
	c.Assert(value, check.Equals, "master")
}

func (s *S) TestValueFieldWhenFieldInvalid(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "", "", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	_, err = ValueField(form, "dleif_dilavni")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid value field \"dleif_dilavni\"")
}

func (s *S) TestValueFieldWhenFieldEmpty(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{
		"branch": "",
	}
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "", "", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	_, err = ValueField(form, "branch")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Empty value \"branch\"")
}

func (s *S) TestFileField(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	var files = []File{
		{"doge.txt", "Much doge"},
		{"much.txt", "Much mucho"},
		{"WOW/WOW.WOW1", "WOW\nWOW"},
		{"WOW/WOW.WOW2", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW3", "WOW\nWOW"},
		{"/usr/WOW/WOW.WOW4", "WOW\nWOW"},
	}
	buf, err := CreateZipBuffer(files)
	c.Assert(err, check.IsNil)
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "muchfile", "muchfile.zip", boundary, writer, buf)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	file, err := FileField(form, "muchfile")
	c.Assert(err, check.IsNil)
	c.Assert(file.Filename, check.Equals, "muchfile.zip")
}

func (s *S) TestFileFieldWhenFieldInvalid(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{
		"dleif_dilavni": "dleif_dilavni",
	}
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "", "", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	_, err = FileField(form, "dleif_dilavni")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid file field \"dleif_dilavni\"")
}

func (s *S) TestFileFieldWhenFieldEmpty(c *check.C) {
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	reader, writer := io.Pipe()
	go StreamWriteMultipartForm(params, "muchfile", "muchfile.zip", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	file, err := FileField(form, "muchfile")
	c.Assert(err, check.IsNil)
	c.Assert(file.Filename, check.Equals, "muchfile.zip")
	fp, err := file.Open()
	c.Assert(err, check.IsNil)
	fs, err := fp.Seek(0, 2)
	c.Assert(err, check.IsNil)
	c.Assert(fs, check.Equals, int64(0))
}
