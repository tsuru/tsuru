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

func (p *dockerProvisioner) migrateImages() error {
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
	dcluster := p.getCluster()
	for _, app := range apps {
		oldImage := registry + repoNamespace + "/" + app.Name
		newImage := registry + repoNamespace + "/app-" + app.Name
		opts := docker.TagImageOptions{Repo: newImage, Force: true}
		err = dcluster.TagImage(oldImage, opts)
		var baseErr error
		if nodeErr, ok := err.(cluster.DockerNodeError); ok {
			baseErr = nodeErr.BaseError()
		}
		if err != nil {
			if err == storage.ErrNoSuchImage || baseErr == docker.ErrNoSuchImage {
				continue
			}
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

func imageCustomDataColl() (*dbStorage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		return nil, err
	}
	return conn.Collection(fmt.Sprintf("%s_image_custom_data", name)), nil
}

func saveImageCustomData(imageName string, customData map[string]interface{}) error {
	coll, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Insert(bson.M{"_id": imageName, "customdata": customData})
}

func getImageTsuruYamlData(imageName string) (provision.TsuruYamlData, error) {
	var customData struct {
		Customdata provision.TsuruYamlData
	}
	coll, err := imageCustomDataColl()
	if err != nil {
		return customData.Customdata, err
	}
	defer coll.Close()
	err = coll.FindId(imageName).One(&customData)
	return customData.Customdata, err
}

// TODO(cezarsa): This method only exist to keep tsuru compatible with older
// platforms. It should be deprecated in the next major after 0.10.0.
func getImageTsuruYamlDataWithFallback(imageName, appName string) (provision.TsuruYamlData, error) {
	yamlData, err := getImageTsuruYamlData(imageName)
	if err != nil {
		a, err := app.GetByName(appName)
		if err != nil {
			// Ignored error if app not found
			return yamlData, nil
		}
		return a.GetTsuruYamlData()
	}
	return yamlData, nil
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
		if err == mgo.ErrNotFound {
			return []string{}, nil
		}
		return nil, err
	}
	historySize := imageHistorySize()
	if len(img.Images) > historySize {
		img.Images = img.Images[len(img.Images)-historySize:]
	}
	return img.Images, nil
}

func isValidAppImage(appName, imageId string) (bool, error) {
	images, err := listValidAppImages(appName)
	if err != nil && err != mgo.ErrNotFound {
		return false, err
	}
	for _, img := range images {
		if img == imageId {
			return true, nil
		}
	}
	return false, nil
}

func imageHistorySize() int {
	imgHistorySize, _ := config.GetInt("docker:image-history-size")
	if imgHistorySize == 0 {
		imgHistorySize = 10
	}
	return imgHistorySize
}

func deleteAllAppImageNames(appName string) error {
	images, err := listAppImages(appName)
	if err != nil {
		return err
	}
	dataColl, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer dataColl.Close()
	_, err = dataColl.RemoveAll(bson.M{"_id": bson.M{"$in": images}})
	if err != nil {
		return err
	}
	coll, err := appImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.RemoveId(appName)
}

func pullAppImageNames(appName string, images []string) error {
	dataColl, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer dataColl.Close()
	_, err = dataColl.RemoveAll(bson.M{"_id": bson.M{"$in": images}})
	if err != nil {
		return err
	}
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

func (p *dockerProvisioner) cleanImage(appName, imgName string) {
	shouldRemove := true
	err := p.getCluster().RemoveImage(imgName)
	if err != nil {
		shouldRemove = false
		log.Errorf("Ignored error removing old image %q: %s. Image kept on list to retry later.",
			imgName, err.Error())
	}
	err = p.getCluster().RemoveFromRegistry(imgName)
	if err != nil {
		shouldRemove = false
		log.Errorf("Ignored error removing old image from registry %q: %s. Image kept on list to retry later.",
			imgName, err.Error())
	}
	if shouldRemove {
		err = pullAppImageNames(appName, []string{imgName})
		if err != nil {
			log.Errorf("Ignored error pulling old images from database: %s", err.Error())
		}
	}
}
