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
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"labix.org/v2/mgo/bson"
)

func imagesToFlatten() []string {
	var apps []app.App
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Caught error while connecting with database: %s", err.Error())
		return nil
	}
	filter := bson.M{"deploys": bson.M{"$mod": []int{20, 0}}}
	if err := conn.Apps().Find(filter).Select(bson.M{"name": 1, "framework": 1}).All(&apps); err != nil {
		log.Fatalf("Caught error while getting apps from database: %s", err.Error())
		return nil
	}
	images := make([]string, len(apps))
	for i, a := range apps {
		images[i] = getImage(&a)
	}
	return images
}

func flatten(image string) error {
	config := docker.Config{
		Image:        image,
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
	opts := dcli.ImportImageOptions{Repository: image, Source: "-"}
	if err := dockerCluster().ImportImage(opts, buf); err != nil {
		log.Printf("Flatten: Caugh error while importing image from container %s: %s", c.ID, err.Error())
		return err
	}
	if err := dockerCluster().RemoveContainer(c.ID); err != nil {
		log.Printf("Flatten: Caugh error while removing container %s: %s", c.ID, err.Error())
	}
	//remove from registry
	return nil
}

// Flatten finds the images that need to be flattened and export/import
// them in order to flatten them. Logs eventual errors.
func Flatten() {
	images := imagesToFlatten()
	for _, image := range images {
		if err := flatten(image); err != nil {
			log.Printf("Flatten: Caugh error while flattening image %s: %s", image, err.Error())
		}
	}
}
