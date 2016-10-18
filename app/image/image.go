// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v1"
)

type ImageMetadata struct {
	Name        string `bson:"_id"`
	CustomData  map[string]interface{}
	Processes   map[string]string
	ExposedPort string
}

type appImages struct {
	AppName string `bson:"_id"`
	Images  []string
	Count   int
}

var procfileRegex = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.+)$`)
var ErrNoImagesAvailable = errors.New("no images available for app")

// GetBuildImage returns the image name from app or plaftorm.
// the platform image will be returned if:
// * there are no containers;
// * the container have an empty image name;
// * the deploy number is multiple of 10.
// in all other cases the app image name will be returne.
func GetBuildImage(app provision.App) string {
	if usePlatformImage(app) {
		return PlatformImageName(app.GetPlatform())
	}
	appImageName, err := AppCurrentImageName(app.GetName())
	if err != nil {
		return PlatformImageName(app.GetPlatform())
	}
	return appImageName
}

func SaveImageCustomData(imageName string, customData map[string]interface{}) error {
	coll, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	var processes map[string]string
	if data, ok := customData["processes"]; ok {
		procs := data.(map[string]interface{})
		processes = make(map[string]string, len(procs))
		for name, command := range procs {
			processes[name] = command.(string)
		}
		delete(customData, "processes")
		delete(customData, "procfile")
	}
	if data, ok := customData["procfile"]; ok {
		procfile := data.(string)
		err := yaml.Unmarshal([]byte(procfile), &processes)
		if err != nil || len(processes) == 0 {
			return errors.New("invalid Procfile")
		}
		delete(customData, "procfile")
	}
	data := ImageMetadata{
		Name:       imageName,
		CustomData: customData,
		Processes:  processes,
	}
	if exposedPort, ok := customData["exposedPort"]; ok {
		data.ExposedPort = exposedPort.(string)
	}
	return coll.Insert(data)
}

func GetImageCustomData(imageName string) (ImageMetadata, error) {
	coll, err := imageCustomDataColl()
	if err != nil {
		return ImageMetadata{}, err
	}
	defer coll.Close()
	var data ImageMetadata
	err = coll.FindId(imageName).One(&data)
	if err == mgo.ErrNotFound {
		// Return empty data for compatibillity with really old apps.
		return data, nil
	}
	return data, err
}

func GetImageWebProcessName(imageName string) (string, error) {
	processName := "web"
	data, err := GetImageCustomData(imageName)
	if err != nil {
		return processName, err
	}
	if len(data.Processes) == 0 {
		return "", nil
	}
	if len(data.Processes) == 1 {
		for name := range data.Processes {
			processName = name
		}
	}
	return processName, nil
}

func GetImageTsuruYamlData(imageName string) (provision.TsuruYamlData, error) {
	var customData struct {
		Customdata provision.TsuruYamlData
	}
	coll, err := imageCustomDataColl()
	if err != nil {
		return customData.Customdata, err
	}
	defer coll.Close()
	err = coll.FindId(imageName).One(&customData)
	if err == mgo.ErrNotFound {
		return customData.Customdata, nil
	}
	return customData.Customdata, err
}

func AppNewImageName(appName string) (string, error) {
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
	return fmt.Sprintf("%s:v%d", appBasicImageName(appName), imgs.Count), nil
}

func AppCurrentImageName(appName string) (string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs appImages
	err = coll.FindId(appName).One(&imgs)
	if err != nil {
		log.Errorf("Couldn't find images for app %q, fallback to old image names. Error: %s", appName, err)
		return appBasicImageName(appName), nil
	}
	if len(imgs.Images) == 0 && imgs.Count > 0 {
		log.Errorf("Couldn't find valid images for app %q", appName)
		return appBasicImageName(appName), nil
	}
	if len(imgs.Images) == 0 {
		return "", ErrNoImagesAvailable
	}
	return imgs.Images[len(imgs.Images)-1], nil
}

func AppendAppImageName(appName, imageId string) error {
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

func ListAppImages(appName string) ([]string, error) {
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

func ListValidAppImages(appName string) ([]string, error) {
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
	historySize := ImageHistorySize()
	if len(img.Images) > historySize {
		img.Images = img.Images[len(img.Images)-historySize:]
	}
	return img.Images, nil
}

func ImageHistorySize() int {
	imgHistorySize, _ := config.GetInt("docker:image-history-size")
	if imgHistorySize == 0 {
		imgHistorySize = 10
	}
	return imgHistorySize
}

func DeleteAllAppImageNames(appName string) error {
	dataColl, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer dataColl.Close()
	_, err = dataColl.RemoveAll(bson.M{"_id": bson.RegEx{
		Pattern: appBasicImageName(appName) + `:v\d+$`,
	}})
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

func PullAppImageNames(appName string, images []string) error {
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

func PlatformImageName(platformName string) string {
	return fmt.Sprintf("%s/%s:latest", basicImageName(), platformName)
}

func GetProcessesFromProcfile(strProcfile string) map[string]string {
	processes := map[string]string{}
	procfile := strings.Split(strProcfile, "\n")
	for _, process := range procfile {
		if p := procfileRegex.FindStringSubmatch(process); p != nil {
			processes[p[1]] = strings.Trim(p[2], " ")
		}
	}
	return processes
}

func CreateImageMetadata(imageName string, processes map[string]string) ImageMetadata {
	customProcesses := map[string]interface{}{}
	for k, v := range processes {
		customProcesses[k] = v
	}
	customData := map[string]interface{}{
		"processes": customProcesses,
	}
	return ImageMetadata{Name: imageName, CustomData: customData, Processes: processes}
}

func appBasicImageName(appName string) string {
	return fmt.Sprintf("%s/app-%s", basicImageName(), appName)
}

func basicImageName() string {
	parts := make([]string, 0, 2)
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	if repoNamespace == "" {
		repoNamespace = "tsuru"
	}
	parts = append(parts, repoNamespace)
	return strings.Join(parts, "/")
}

func usePlatformImage(app provision.App) bool {
	maxLayers, _ := config.GetUint("docker:max-layers")
	if maxLayers == 0 {
		maxLayers = 10
	}
	deploys := app.GetDeploys()
	return deploys%maxLayers == 0 || app.GetUpdatePlatform()
}

func appImagesColl() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = "docker"
	}
	return conn.Collection(fmt.Sprintf("%s_app_image", name)), nil
}

func imageCustomDataColl() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = "docker"
	}
	return conn.Collection(fmt.Sprintf("%s_image_custom_data", name)), nil
}
