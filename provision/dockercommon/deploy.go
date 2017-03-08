// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"archive/tar"
	"io"

	"github.com/pkg/errors"
)

func writeTarball(tarball *tar.Writer, archive io.Reader, fileSize int64, name string) error {
	header := tar.Header{
		Name: name,
		Mode: 0666,
		Size: fileSize,
	}
	tarball.WriteHeader(&header)
	n, err := io.Copy(tarball, archive)
	if err != nil {
		return err
	}
	if n != fileSize {
		return errors.New("upload-deploy: short-write copying to tarball")
	}
	return tarball.Close()
}

func AddDeployTarFile(archive io.Reader, fileSize int64, name string) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		tarball := tar.NewWriter(writer)
		err := writeTarball(tarball, archive, fileSize, name)
		if err != nil {
			writer.CloseWithError(err)
			tarball.Close()
		} else {
			writer.Close()
		}
	}()
	return reader
}
