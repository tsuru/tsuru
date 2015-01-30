// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	dbStorage "github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type appImages struct {
	AppName string `bson:"_id"`
	Images  []string
	Count   int
}

func migrateImages() error {
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		registry += "/"
	}
	repoNamespace, err := config.GetString("docker:repository-namespace")
	if err != nil {
		return err
	}
	apps, err := app.List(nil)
	if err != nil {
		return err
	}
	dcluster := dockerCluster()
	for _, app := range apps {
		oldImage := registry + repoNamespace + "/" + app.Name
		newImage := registry + repoNamespace + "/app-" + app.Name
		opts := docker.TagImageOptions{Repo: newImage, Force: true}
		err = dcluster.TagImage(oldImage, opts)
		var baseErr error
		if nodeErr, ok := err.(cluster.DockerNodeError); ok {
			baseErr = nodeErr.BaseError()
		}
		if err != nil && err != storage.ErrNoSuchImage && baseErr != docker.ErrNoSuchImage {
			return err
		}
		if registry != "" {
			pushOpts := docker.PushImageOptions{Name: newImage}
			err = dcluster.PushImage(pushOpts, docker.AuthConfiguration{})
			if err != nil {
				return err
			}
		}
		err = updateContainers(bson.M{"appname": app.Name}, bson.M{"$set": bson.M{"image": newImage}})
		if err != nil {
			return err
		}
	}
	return nil
}

// getBuildImage returns the image name from app or plaftorm.
// the platform image will be returned if:
// * there are no containers;
// * the container have an empty image name;
// * the deploy number is multiple of 10.
// in all other cases the app image name will be returne.
func getBuildImage(app provision.App) string {
	if usePlatformImage(app) {
		return platformImageName(app.GetPlatform())
	}
	appImageName, err := appCurrentImageName(app.GetName())
	if err != nil {
		return platformImageName(app.GetPlatform())
	}
	return appImageName
}

func appImagesColl() (*dbStorage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		return nil, err
	}
	return conn.Collection(fmt.Sprintf("%s_app_image", name)), nil
}

func appNewImageName(appName string) (string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs appImages
	dbChange := mgo.Change{
		Update:    bson.M{"$inc": bson.M{"count": 1}},
		ReturnNew: true,
		Upsert:    true,
	}
	_, err = coll.FindId(appName).Apply(dbChange, &imgs)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/app-%s:v%d", basicImageName(), appName, imgs.Count), nil
}

func appCurrentImageName(appName string) (string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs appImages
	err = coll.FindId(appName).One(&imgs)
	if err != nil {
		log.Errorf("Couldn't find images for app %q, fallback to old image names. Error: %s", appName, err.Error())
		return fmt.Sprintf("%s/app-%s", basicImageName(), appName), nil
	}
	if len(imgs.Images) == 0 {
		return "", fmt.Errorf("no images available for app %q", appName)
	}
	return imgs.Images[len(imgs.Images)-1], nil
}

func appendAppImageName(appName, imageId string) error {
	coll, err := appImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(appName, bson.M{"$pull": bson.M{"images": imageId}})
	if err != nil {
		return err
	}
	_, err = coll.UpsertId(appName, bson.M{"$push": bson.M{"images": imageId}})
	return err
}

func listAppImages(appName string) ([]string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var imgs appImages
	err = coll.FindId(appName).One(&imgs)
	if err != nil {
		return nil, err
	}
	return imgs.Images, nil
}

func listValidAppImages(appName string) ([]string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var img appImages
	err = coll.FindId(appName).One(&img)
	if err != nil {
		return nil, err
	}
	historySize := imageHistorySize()
	if len(img.Images) > historySize {
		img.Images = img.Images[len(img.Images)-historySize:]
	}
	return img.Images, nil
}

func imageHistorySize() int {
	imgHistorySize, _ := config.GetInt("docker:image-history-size")
	if imgHistorySize == 0 {
		imgHistorySize = 10
	}
	return imgHistorySize
}

func deleteAllAppImageNames(appName string) error {
	coll, err := appImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.RemoveId(appName)
}

func pullAppImageNames(appName string, images []string) error {
	coll, err := appImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(appName, bson.M{"$pullAll": bson.M{"images": images}})
}

func platformImageName(platformName string) string {
	return fmt.Sprintf("%s/%s", basicImageName(), platformName)
}

func basicImageName() string {
	parts := make([]string, 0, 2)
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	parts = append(parts, repoNamespace)
	return strings.Join(parts, "/")
}

func usePlatformImage(app provision.App) bool {
	deploys := app.GetDeploys()
	if (deploys != 0 && deploys%10 == 0) || app.GetUpdatePlatform() {
		return true
	}
	c, err := getOneContainerByAppName(app.GetName())
	if err != nil || c.Image == "" {
		return true
	}
	return false
}
