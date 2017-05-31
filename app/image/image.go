// Copyright 2014 tsuru authors. All rights reserved.
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
)

type ImageMetadata struct {
	Name            string `bson:"_id"`
	CustomData      map[string]interface{}
	LegacyProcesses map[string]string   `bson:"processes"`
	Processes       map[string][]string `bson:"processes_list"`
	ExposedPort     string
}

type appImages struct {
	AppName string `bson:"_id"`
	Images  []string
	Count   int
}

func (i *ImageMetadata) Save() error {
	if i.Name == "" {
		return errors.New("image name is mandatory")
	}
	coll, err := imageCustomDataColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Insert(i)
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

func customDataToImageMetadata(imageName string, customData map[string]interface{}) (*ImageMetadata, error) {
	var processes map[string][]string
	if data, ok := customData["processes"]; ok {
		procs := data.(map[string]interface{})
		processes = make(map[string][]string, len(procs))
		for name, command := range procs {
			switch cmdType := command.(type) {
			case string:
				processes[name] = []string{cmdType}
			case []string:
				processes[name] = cmdType
			case []interface{}:
				for _, v := range cmdType {
					if vStr, ok := v.(string); ok {
						processes[name] = append(processes[name], vStr)
					}
				}
			default:
				return nil, fmt.Errorf("invalid type for process entry for image %q: %T", imageName, cmdType)
			}
		}
		delete(customData, "processes")
		delete(customData, "procfile")
	}
	if data, ok := customData["procfile"]; ok {
		processes = GetProcessesFromProcfile(data.(string))
		if len(processes) == 0 {
			return nil, errors.New("invalid Procfile")
		}
		delete(customData, "procfile")
	}
	data := ImageMetadata{
		Name:       imageName,
		Processes:  processes,
		CustomData: customData,
	}
	if exposedPort, ok := customData["exposedPort"]; ok {
		data.ExposedPort = exposedPort.(string)
	}
	return &data, nil
}

func SaveImageCustomData(imageName string, customData map[string]interface{}) error {
	data, err := customDataToImageMetadata(imageName, customData)
	if err != nil {
		return err
	}
	return data.Save()
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
		// Return empty data for compatibility with really old apps.
		return data, nil
	}
	if len(data.Processes) == 0 {
		data.Processes = make(map[string][]string, len(data.LegacyProcesses))
		for k, v := range data.LegacyProcesses {
			data.Processes[k] = []string{v}
		}
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

func AllAppProcesses(appName string) ([]string, error) {
	var processes []string
	imgID, err := AppCurrentImageName(appName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := GetImageCustomData(imgID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for procName := range data.Processes {
		processes = append(processes, procName)
	}
	return processes, nil
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

func GetProcessesFromProcfile(strProcfile string) map[string][]string {
	procfile := strings.Split(strProcfile, "\n")
	processes := make(map[string][]string, len(procfile))
	for _, process := range procfile {
		if p := procfileRegex.FindStringSubmatch(process); p != nil {
			processes[p[1]] = []string{strings.TrimSpace(p[2])}
		}
	}
	return processes
}

func AppNewBuilderImageName(appName string) (string, error) {
	coll, err := appImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs appImages
	err = coll.FindId(appName).One(&imgs)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return "", err
	}
	version := imgs.Count + 1
	return fmt.Sprintf("%s:v%d-builder", appBasicImageName(appName), version), nil
}

func ListAppBuilderImages(appName string) ([]string, error) {
	coll, err := appBuilderImagesColl()
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

func AppendAppBuilderImageName(appName, imageId string) error {
	coll, err := appBuilderImagesColl()
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

func AppCurrentBuilderImageName(appName string) (string, error) {
	coll, err := appBuilderImagesColl()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	var imgs appImages
	err = coll.FindId(appName).One(&imgs)
	if err != nil {
		log.Errorf("Couldn't find builder images for app %q, fallback to old image names. Error: %s", appName, err)
		return "", nil
	}
	if len(imgs.Images) == 0 {
		log.Errorf("Couldn't find valid images for app %q", appName)
		return "", nil
	}
	if len(imgs.Images) == 0 {
		return "", ErrNoImagesAvailable
	}
	return imgs.Images[len(imgs.Images)-1], nil
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

func appBuilderImagesColl() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("builder_app_image"), nil
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
