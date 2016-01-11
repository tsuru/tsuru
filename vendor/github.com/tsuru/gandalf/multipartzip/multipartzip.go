// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multipartzip

import (
	"archive/zip"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"

	"github.com/tsuru/gandalf/fs"
)

func ValueField(f *multipart.Form, n string) (string, error) {
	v, ok := f.Value[n]
	if !ok {
		return "", fmt.Errorf("Invalid value field %q", n)
	}
	if len(v) != 1 {
		return "", fmt.Errorf("Not a single value %q", n)
	}
	if len(v[0]) == 0 {
		return "", fmt.Errorf("Empty value %q", n)
	}
	return v[0], nil
}

func FileField(f *multipart.Form, n string) (*multipart.FileHeader, error) {
	v, ok := f.File[n]
	if !ok {
		return nil, fmt.Errorf("Invalid file field %q", n)
	}
	if len(v) != 1 {
		return nil, fmt.Errorf("Not a single file %q", n)
	}
	return v[0], nil
}

func CopyZipFile(f *zip.File, d, p string) error {
	if p != "" {
		dirname := path.Dir(p)
		if dirname != "." {
			err := os.MkdirAll(path.Join(d, dirname), 0755)
			if err != nil {
				return err
			}
		}
		rc, err := f.Open()
		defer rc.Close()
		if err != nil {
			return err
		}
		path := path.Join(d, p)
		stat, err := os.Stat(path)
		if err != nil || !stat.IsDir() {
			file, err := fs.Filesystem().Create(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(file, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ExtractZip(f *multipart.FileHeader, d string) error {
	file, err := f.Open()
	defer file.Close()
	if err != nil {
		return err
	}
	size, err := file.Seek(0, 2)
	if err != nil {
		return err
	}
	r, err := zip.NewReader(file, size)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		err := CopyZipFile(f, d, f.Name)
		if err != nil {
			return err
		}
	}
	return nil
}
