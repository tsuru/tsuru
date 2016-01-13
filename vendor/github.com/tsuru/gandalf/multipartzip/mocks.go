// Copyright 2014 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multipartzip

import (
	"archive/zip"
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
)

type File struct {
	Name string
	Body string
}

func CreateZipBuffer(files []File) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, file := range files {
		f, err := w.Create(file.Name)
		if err != nil {
			return nil, err
		}
		_, err = f.Write([]byte(file.Body))
		if err != nil {
			return nil, err
		}
	}
	err := w.Close()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func StreamWriteMultipartForm(params map[string]string, fileField, path, boundary string, pw *io.PipeWriter, buf *bytes.Buffer) {
	defer pw.Close()
	mpw := multipart.NewWriter(pw)
	mpw.SetBoundary(boundary)
	if fileField != "" && path != "" {
		fw, err := mpw.CreateFormFile(fileField, filepath.Base(path))
		if err != nil {
			log.Fatal(err)
			return
		}
		if buf != nil {
			_, err = io.Copy(fw, buf)
			if err != nil {
				log.Fatal(err)
				return
			}
		}
	}
	for key, val := range params {
		_ = mpw.WriteField(key, val)
	}
	err := mpw.Close()
	if err != nil {
		log.Fatal(err)
		return
	}
}
