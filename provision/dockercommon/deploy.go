// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockercommon

import (
	"archive/tar"
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

const (
	defaultUsername = "ubuntu"
	defaultUserID   = 1000
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

func UserForContainer() (username string, uid *int64) {
	userid, err := config.GetInt("docker:uid")
	if err == nil {
		if userid >= 0 {
			userid64 := int64(userid)
			uid = &userid64
		}
	} else {
		defUID := int64(defaultUserID)
		uid = &defUID
	}
	username, err = config.GetString("docker:user")
	if err != nil {
		username, err = config.GetString("docker:ssh:user")
		if err != nil {
			username = defaultUsername
		}
	}
	return username, uid
}

func DeployHealthcheckTimeout(tsuruYamlData provTypes.TsuruYamlData) time.Duration {
	const defaultWaitSeconds = 120

	minWaitSeconds, _ := config.GetInt("docker:healthcheck:max-time")
	if minWaitSeconds <= 0 {
		minWaitSeconds = defaultWaitSeconds
	}

	var waitTime int
	if tsuruYamlData.Healthcheck != nil {
		waitTime = tsuruYamlData.Healthcheck.DeployTimeoutSeconds
	}
	if waitTime < minWaitSeconds {
		waitTime = minWaitSeconds
	}

	return time.Duration(waitTime) * time.Second
}
