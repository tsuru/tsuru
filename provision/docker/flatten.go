// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Finds tsuru applications which deploys count % 20 == 0 |||| this is wrong! if count is 30 % 20 will be 10 and the app still needs a flatten!
// and flatten their filesystems in order to avoid aufs performance bottlenecks.
package docker

import (
	"bytes"
	"github.com/dotcloud/docker"
	dcli "github.com/fsouza/go-dockerclient"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
)

func imageToFlatten(a *app.App) string {
	if a.Deploys%20 == 0 {
		return getImage(a)
	}
	return ""
}

func flatten(imageID string) error {
	config := docker.Config{
		Image:        imageID,
		Cmd:          []string{"/bin/bash"},
		AttachStdin:  false,
		AttachStdout: false,
		AttachStderr: false,
	}
	_, c, err := dockerCluster().CreateContainer(&config)
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	if err := dockerCluster().ExportContainer(c.ID, buf); err != nil {
		log.Printf("Flatten: Caugh error while exporting container %s: %s", c.ID, err.Error())
		return err
	}
	opts := dcli.ImportImageOptions{Repository: imageID, Source: "-"}
	if err := dockerCluster().ImportImage(opts, buf); err != nil {
		log.Printf("Flatten: Caugh error while importing image from container %s: %s", c.ID, err.Error())
		return err
	}
	if err := dockerCluster().RemoveContainer(c.ID); err != nil {
		log.Printf("Flatten: Caugh error while removing container %s: %s", c.ID, err.Error())
	}
	removeFromRegistry(imageID)
	return nil
}

// Flatten finds the images that need to be flattened and export/import
// them in order to flatten them and logs errors when they happen.
func Flatten(a *app.App) {
	image := imageToFlatten(a)
	if image != "" {
		if err := flatten(image); err != nil {
			log.Printf("Flatten: Caugh error while flattening image %s: %s", image, err.Error())
		}
	}
}
