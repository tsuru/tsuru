// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/servicemanager"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

const defaultCollection = "docker"

var (
	ErrNoImagesAvailable = errors.New("no images available for app")
	procfileRegex        = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.+)$`)
)

type ImageNotFoundErr struct {
	App, Image string
}

type customData struct {
	Hooks       *provTypes.TsuruYamlHooks       `bson:",omitempty"`
	Healthcheck *provTypes.TsuruYamlHealthcheck `bson:",omitempty"`
	Kubernetes  *tsuruYamlKubernetesConfig      `bson:",omitempty"`
}

type tsuruYamlKubernetesConfig struct {
	Groups []tsuruYamlKubernetesGroup `bson:",omitempty"`
}

type tsuruYamlKubernetesGroup struct {
	Name      string
	Processes []tsuruYamlKubernetesProcess `bson:",omitempty"`
}

type tsuruYamlKubernetesProcess struct {
	Name  string
	Ports []tsuruYamlKubernetesProcessPortConfig `bson:",omitempty"`
}

type tsuruYamlKubernetesProcessPortConfig struct {
	Name       string `json:"name,omitempty" bson:"name,omitempty"`
	Protocol   string `json:"protocol,omitempty" bson:"protocol,omitempty"`
	Port       int    `json:"port,omitempty" bson:"port,omitempty"`
	TargetPort int    `json:"target_port,omitempty" bson:"target_port,omitempty"`
}

func (i *ImageNotFoundErr) Error() string {
	return fmt.Sprintf("Image %s not found in app %q", i.Image, i.App)
}

type InvalidVersionErr struct {
	Image string
}

func (i *InvalidVersionErr) Error() string {
	return fmt.Sprintf("Invalid version: %s", i.Image)
}

type ImageMetadata struct {
	Name            string `bson:"_id"`
	CustomData      map[string]interface{}
	LegacyProcesses map[string]string   `bson:"processes"`
	Processes       map[string][]string `bson:"processes_list"`
	ExposedPorts    []string
	DisableRollback bool
	Reason          string
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
	customData, err := marshalCustomData(i.CustomData)
	if err != nil {
		return err
	}
	newImg := ImageMetadata{
		Name:            i.Name,
		CustomData:      customData,
		LegacyProcesses: i.LegacyProcesses,
		Processes:       i.Processes,
		ExposedPorts:    i.ExposedPorts,
		DisableRollback: i.DisableRollback,
		Reason:          i.Reason,
	}

	coll, err := ImageCustomDataColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Insert(newImg)
}

// GetBuildImage returns the image name from app or plaftorm.
// the platform image will be returned if:
// * there are no containers;
// * the container have an empty image name;
// * the deploy number is multiple of 10.
// in all other cases the app image name will be returned.
func GetBuildImage(app provision.App) (string, error) {
	if usePlatformImage(app) {
		return getPlatformImage(app)
	}
	appImageName, err := AppCurrentImageName(app.GetName())
	if err != nil {
		return getPlatformImage(app)
	}
	return appImageName, nil
}

func getPlatformImage(app provision.App) (string, error) {
	version := app.GetPlatformVersion()
	if version != "latest" {
		return servicemanager.PlatformImage.FindImage(app.GetPlatform(), version)
	}
	return servicemanager.PlatformImage.CurrentImage(app.GetPlatform())
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
	return &data, nil
}

func SaveImageCustomData(imageName string, customData map[string]interface{}) error {
	data, err := customDataToImageMetadata(imageName, customData)
	if err != nil {
		return err
	}
	return data.Save()
}

func GetImageMetaData(imageName string) (ImageMetadata, error) {
	coll, err := ImageCustomDataColl()
	if err != nil {
		return ImageMetadata{}, err
	}
	defer coll.Close()
	var data ImageMetadata
	err = coll.FindId(imageName).One(&data)
	if err == mgo.ErrNotFound {
		// Return empty data for compatibility with really old apps.
		return ImageMetadata{}, nil
	}
	if len(data.Processes) == 0 {
		data.Processes = make(map[string][]string, len(data.LegacyProcesses))
		for k, v := range data.LegacyProcesses {
			data.Processes[k] = []string{v}
		}
	}
	customData, err := unmarshalCustomData(data.CustomData)
	if err != nil {
		return ImageMetadata{}, err
	}
	b, err := json.Marshal(customData)
	if err != nil {
		return ImageMetadata{}, err
	}
	var jsonData map[string]interface{}
	err = json.Unmarshal(b, &jsonData)
	if err != nil {
		return ImageMetadata{}, err
	}
	data.CustomData = jsonData
	return data, nil
}

func GetImageWebProcessName(imageName string) (string, error) {
	data, err := GetImageMetaData(imageName)
	if err != nil {
		return "", err
	}
	if len(data.Processes) == 0 {
		return "", nil
	}
	var processes []string
	for name := range data.Processes {
		if name == "web" || len(data.Processes) == 1 {
			return name, nil
		}
		processes = append(processes, name)
	}
	sort.Strings(processes)
	return processes[0], nil
}

func AllAppProcesses(appName string) ([]string, error) {
	var processes []string
	imgID, err := AppCurrentImageName(appName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := GetImageMetaData(imgID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for procName := range data.Processes {
		processes = append(processes, procName)
	}
	return processes, nil
}

func GetImageTsuruYamlData(imageName string) (provTypes.TsuruYamlData, error) {
	var data struct {
		Customdata map[string]interface{}
	}
	coll, err := ImageCustomDataColl()
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}
	defer coll.Close()
	err = coll.FindId(imageName).One(&data)
	if err == mgo.ErrNotFound {
		return provTypes.TsuruYamlData{}, nil
	}
	return unmarshalYamlData(data.Customdata)
}

func marshalCustomData(data map[string]interface{}) (map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var yamlData provTypes.TsuruYamlData
	err = json.Unmarshal(b, &yamlData)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for k, v := range data {
		if v != nil {
			result[k] = v
		}
	}
	result["hooks"] = yamlData.Hooks
	result["healthcheck"] = yamlData.Healthcheck
	if yamlData.Kubernetes == nil {
		return result, nil
	}
	kubeConfig := &tsuruYamlKubernetesConfig{}

	for groupName, groupData := range yamlData.Kubernetes.Groups {
		group := tsuruYamlKubernetesGroup{Name: groupName}
		for procName, procData := range groupData {
			proc := tsuruYamlKubernetesProcess{Name: procName}
			for _, port := range procData.Ports {
				proc.Ports = append(proc.Ports, tsuruYamlKubernetesProcessPortConfig(port))
			}
			group.Processes = append(group.Processes, proc)
		}
		if kubeConfig.Groups == nil {
			kubeConfig.Groups = []tsuruYamlKubernetesGroup{group}
		} else {
			kubeConfig.Groups = append(kubeConfig.Groups, group)
		}
	}
	result["kubernetes"] = kubeConfig
	return result, nil
}

func unmarshalCustomData(data map[string]interface{}) (map[string]interface{}, error) {
	if data == nil {
		return nil, nil
	}

	yamlData, err := unmarshalYamlData(data)
	if err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	for k, v := range data {
		if v != nil {
			result[k] = v
		}
	}
	if yamlData.Hooks != nil {
		result["hooks"] = yamlData.Hooks
	}
	if yamlData.Healthcheck != nil {
		result["healthcheck"] = yamlData.Healthcheck
	}
	if yamlData.Kubernetes != nil {
		result["kubernetes"] = yamlData.Kubernetes
	}
	return result, nil
}

func unmarshalYamlData(data map[string]interface{}) (provTypes.TsuruYamlData, error) {
	if data == nil {
		return provTypes.TsuruYamlData{}, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}
	custom := customData{}
	err = json.Unmarshal(b, &custom)
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}

	result := provTypes.TsuruYamlData{
		Hooks:       custom.Hooks,
		Healthcheck: custom.Healthcheck,
	}
	if custom.Kubernetes == nil {
		return result, nil
	}

	result.Kubernetes = &provTypes.TsuruYamlKubernetesConfig{}
	for _, g := range custom.Kubernetes.Groups {
		group := provTypes.TsuruYamlKubernetesGroup{}
		for _, proc := range g.Processes {
			group[proc.Name] = provTypes.TsuruYamlKubernetesProcessConfig{
				Ports: make([]provTypes.TsuruYamlKubernetesProcessPortConfig, len(proc.Ports)),
			}
			for i, port := range proc.Ports {
				group[proc.Name].Ports[i] = provTypes.TsuruYamlKubernetesProcessPortConfig(port)
			}
		}
		if result.Kubernetes.Groups == nil {
			result.Kubernetes.Groups = map[string]provTypes.TsuruYamlKubernetesGroup{
				g.Name: group,
			}
		} else {
			result.Kubernetes.Groups[g.Name] = group
		}
	}
	return result, nil
}

func AppNewBuildImageName(appName, teamName, customTag string) (NewImageInfo, error) {
	return appNewImageName(appName, teamName, customTag, true)
}

func AppNewImageName(appName string) (NewImageInfo, error) {
	return appNewImageName(appName, "", "", false)
}

func appNewImageName(appName, teamName, customTag string, isBuild bool) (NewImageInfo, error) {
	coll, err := appImagesColl()
	if err != nil {
		return NewImageInfo{}, err
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
		return NewImageInfo{}, err
	}
	return NewImageInfo{
		appName:   appName,
		version:   imgs.Count,
		customTag: customTag,
		teamName:  teamName,
		isBuild:   isBuild,
	}, nil
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

func AppendAppImageName(appName, imageID string) error {
	coll, err := appImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	bulk := coll.Bulk()
	bulk.Upsert(bson.M{"_id": appName}, bson.M{"$pull": bson.M{"images": imageID}})
	bulk.Upsert(bson.M{"_id": appName}, bson.M{"$push": bson.M{"images": imageID}})
	_, err = bulk.Run()
	return err
}

type AllAppImages struct {
	DeployImages  []string
	BuilderImages []string
}

func ListAllAppImages() (map[string]AllAppImages, error) {
	coll, err := appImagesColl()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var (
		imgsDeploy []appImages
		imgsBuild  []appImages
	)
	err = coll.Find(nil).All(&imgsDeploy)
	if err != nil {
		return nil, err
	}
	coll, err = appBuilderImagesColl()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	err = coll.Find(nil).All(&imgsBuild)
	if err != nil {
		return nil, err
	}
	ret := map[string]AllAppImages{}
	for _, img := range imgsDeploy {
		appData := ret[img.AppName]
		appData.DeployImages = img.Images
		ret[img.AppName] = appData
	}
	for _, img := range imgsBuild {
		appData := ret[img.AppName]
		appData.BuilderImages = img.Images
		ret[img.AppName] = appData
	}
	return ret, nil
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
	dataColl, err := ImageCustomDataColl()
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
	err = coll.RemoveId(appName)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	coll, err = appBuilderImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(appName)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return nil
}

func UpdateAppImageRollback(img, reason string, disableRollback bool) error {
	dataColl, err := ImageCustomDataColl()
	if err != nil {
		return err
	}
	defer dataColl.Close()
	return dataColl.Update(bson.M{"_id": img}, bson.M{"$set": bson.M{"disablerollback": disableRollback, "reason": reason}})
}

func PullAppImageNames(appName string, images []string) error {
	dataColl, err := ImageCustomDataColl()
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
	err = coll.UpdateId(appName, bson.M{"$pullAll": bson.M{"images": images}})
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	coll, err = appBuilderImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(appName, bson.M{"$pullAll": bson.M{"images": images}})
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return nil
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

func AppendAppBuilderImageName(appName, imageID string) error {
	coll, err := appBuilderImagesColl()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(appName, bson.M{"$pull": bson.M{"images": imageID}})
	if err != nil {
		return err
	}
	_, err = coll.UpsertId(appName, bson.M{"$push": bson.M{"images": imageID}})
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

func GetAppImageBySuffix(appName, imageIdSuffix string) (string, error) {
	inputImage := imageIdSuffix
	validImgs, err := ListValidAppImages(appName)
	if err != nil {
		return "", err
	}
	if len(validImgs) == 0 {
		return "", &ImageNotFoundErr{App: appName, Image: inputImage}
	}
	for _, img := range validImgs {
		if strings.HasSuffix(img, inputImage) {
			return img, nil
		}
	}
	return "", &InvalidVersionErr{Image: inputImage}
}

func SplitImageName(imageName string) (repo, tag string) {
	imgNameSplit := strings.Split(imageName, ":")
	switch len(imgNameSplit) {
	case 1:
		repo = imgNameSplit[0]
		tag = "latest"
	case 2:
		if strings.Contains(imgNameSplit[1], "/") {
			repo = imageName
			tag = "latest"
		} else {
			repo = imgNameSplit[0]
			tag = imgNameSplit[1]
		}
	default:
		repo = strings.Join(imgNameSplit[:len(imgNameSplit)-1], ":")
		tag = imgNameSplit[len(imgNameSplit)-1]
	}
	return
}

func appBasicImageName(appName string) string {
	return fmt.Sprintf("%s/app-%s", basicImageName("tsuru"), appName)
}

func appBasicBuilderImageName(appName, teamName string) string {
	if teamName == "" {
		teamName = "tsuru"
	}
	return fmt.Sprintf("%s/app-%s", basicImageName(teamName), appName)
}

func basicImageName(repoName string) string {
	parts := make([]string, 0, 2)
	registry, _ := config.GetString("docker:registry")
	if registry != "" {
		parts = append(parts, registry)
	}
	repoNamespace, _ := config.GetString("docker:repository-namespace")
	if repoNamespace == "" {
		repoNamespace = repoName
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
		name = defaultCollection
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

func ImageCustomDataColl() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		name = defaultCollection
	}
	return conn.Collection(fmt.Sprintf("%s_image_custom_data", name)), nil
}

type NewImageInfo struct {
	appName   string
	teamName  string
	customTag string
	version   int
	isBuild   bool
}

func (img NewImageInfo) BaseImageName() string {
	return fmt.Sprintf("%s:v%d", appBasicImageName(img.appName), img.version)
}

func (img NewImageInfo) BuildImageName() string {
	if !img.isBuild {
		return ""
	}
	tag := img.customTag
	if tag == "" {
		tag = fmt.Sprintf("v%d-builder", img.version)
	}
	return fmt.Sprintf("%s:%s", appBasicBuilderImageName(img.appName, img.teamName), tag)
}

func (img NewImageInfo) Version() int {
	return img.version
}

func (img NewImageInfo) IsBuild() bool {
	return img.isBuild
}
