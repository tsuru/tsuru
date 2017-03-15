// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image_test

import (
	"sort"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	storage *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "app_image_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Close()
}

func (s *S) TestAppNewImageName(c *check.C) {
	img1, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
	img2, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
	img3, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "tsuru/app-myapp:v3")
}

func (s *S) TestAppNewImageNameWithRegistry(c *check.C) {
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	img1, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "localhost:3030/tsuru/app-myapp:v1")
	img2, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "localhost:3030/tsuru/app-myapp:v2")
	img3, err := image.AppNewImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img3, check.Equals, "localhost:3030/tsuru/app-myapp:v3")
}

func (s *S) TestAppCurrentImageNameWithoutImage(c *check.C) {
	img1, err := image.AppCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp")
}

func (s *S) TestAppendAppImageChangeImagePosition(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	images, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v1"})
}

func (s *S) TestAppCurrentImageName(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	img1, err := image.AppCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img1, check.Equals, "tsuru/app-myapp:v1")
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	img2, err := image.AppCurrentImageName("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(img2, check.Equals, "tsuru/app-myapp:v2")
}

func (s *S) TestListAppImages(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	images, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:v2"})
}

func (s *S) TestValidListAppImages(c *check.C) {
	config.Set("docker:image-history-size", 2)
	defer config.Unset("docker:image-history-size")
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	images, err := image.ListValidAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v3"})
}

func (s *S) TestPlatformImageName(c *check.C) {
	platName := image.PlatformImageName("python")
	c.Assert(platName, check.Equals, "tsuru/python:latest")
	config.Set("docker:registry", "localhost:3030")
	defer config.Unset("docker:registry")
	platName = image.PlatformImageName("ruby")
	c.Assert(platName, check.Equals, "localhost:3030/tsuru/ruby:latest")
}

func (s *S) TestDeleteAllAppImageNames(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = image.DeleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = image.ListAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
}

func (s *S) TestDeleteAllAppImageNamesRemovesCustomData(c *check.C) {
	imgName := "tsuru/app-myapp:v1"
	err := image.AppendAppImageName("myapp", imgName)
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err = image.SaveImageCustomData(imgName, data)
	c.Assert(err, check.IsNil)
	err = image.DeleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = image.ListAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	yamlData, err := image.GetImageTsuruYamlData(imgName)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestDeleteAllAppImageNamesRemovesCustomDataWithoutImages(c *check.C) {
	imgName := "tsuru/app-myapp:v1"
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err := image.SaveImageCustomData(imgName, data)
	c.Assert(err, check.IsNil)
	err = image.DeleteAllAppImageNames("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	yamlData, err := image.GetImageTsuruYamlData(imgName)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestDeleteAllAppImageNamesSimilarApps(c *check.C) {
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("tsuru/app-myapp:v1", data)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp-dev", "tsuru/app-myapp-dev:v1")
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("tsuru/app-myapp-dev:v1", data)
	c.Assert(err, check.IsNil)
	err = image.DeleteAllAppImageNames("myapp")
	c.Assert(err, check.IsNil)
	_, err = image.ListAppImages("myapp")
	c.Assert(err, check.ErrorMatches, "not found")
	_, err = image.ListAppImages("myapp-dev")
	c.Assert(err, check.IsNil)
	yamlData, err := image.GetImageTsuruYamlData("tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
	yamlData, err = image.GetImageTsuruYamlData("tsuru/app-myapp-dev:v1")
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{
		Healthcheck: provision.TsuruYamlHealthcheck{Path: "/test"},
	})
}

func (s *S) TestPullAppImageNames(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	err = image.PullAppImageNames("myapp", []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:v3"})
	c.Assert(err, check.IsNil)
	images, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2"})
}

func (s *S) TestPullAppImageNamesRemovesCustomData(c *check.C) {
	img1Name := "tsuru/app-myapp:v1"
	err := image.AppendAppImageName("myapp", img1Name)
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v2")
	c.Assert(err, check.IsNil)
	err = image.AppendAppImageName("myapp", "tsuru/app-myapp:v3")
	c.Assert(err, check.IsNil)
	data := map[string]interface{}{"healthcheck": map[string]interface{}{"path": "/test"}}
	err = image.SaveImageCustomData(img1Name, data)
	c.Assert(err, check.IsNil)
	err = image.PullAppImageNames("myapp", []string{img1Name})
	c.Assert(err, check.IsNil)
	images, err := image.ListAppImages("myapp")
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"tsuru/app-myapp:v2", "tsuru/app-myapp:v3"})
	yamlData, err := image.GetImageTsuruYamlData(img1Name)
	c.Assert(err, check.IsNil)
	c.Assert(yamlData, check.DeepEquals, provision.TsuruYamlData{})
}

func (s *S) TestGetImageWebProcessName(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "python myapp.py",
			"worker": "someworker",
		},
	}
	err := image.SaveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	img2 := "tsuru/app-myapp:v2"
	customData2 := map[string]interface{}{
		"processes": map[string]interface{}{
			"worker1": "python myapp.py",
			"worker2": "someworker",
		},
	}
	err = image.SaveImageCustomData(img2, customData2)
	c.Assert(err, check.IsNil)
	img3 := "tsuru/app-myapp:v3"
	customData3 := map[string]interface{}{
		"processes": map[string]interface{}{
			"api": "python myapi.py",
		},
	}
	err = image.SaveImageCustomData(img3, customData3)
	c.Assert(err, check.IsNil)
	img4 := "tsuru/app-myapp:v4"
	customData4 := map[string]interface{}{}
	err = image.SaveImageCustomData(img4, customData4)
	c.Assert(err, check.IsNil)
	web1, err := image.GetImageWebProcessName(img1)
	c.Check(err, check.IsNil)
	c.Check(web1, check.Equals, "web")
	web2, err := image.GetImageWebProcessName(img2)
	c.Check(err, check.IsNil)
	c.Check(web2, check.Equals, "web")
	web3, err := image.GetImageWebProcessName(img3)
	c.Check(err, check.IsNil)
	c.Check(web3, check.Equals, "api")
	web4, err := image.GetImageWebProcessName(img4)
	c.Check(err, check.IsNil)
	c.Check(web4, check.Equals, "")
	img5 := "tsuru/app-myapp:v5"
	web5, err := image.GetImageWebProcessName(img5)
	c.Check(err, check.IsNil)
	c.Check(web5, check.Equals, "")
}

func (s *S) TestSavePortInImageCustomData(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"exposedPort": "3434",
	}
	err := image.SaveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	imageMetaData, err := image.GetImageCustomData(img1)
	c.Check(err, check.IsNil)
	c.Check(imageMetaData.ExposedPort, check.Equals, "3434")
}

func (s *S) TestSaveImageCustomData(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"exposedPort": "3434",
		"processes": map[string]interface{}{
			"worker1": "python myapp.py",
			"worker2": "someworker",
		},
	}
	err := image.SaveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	imageMetaData, err := image.GetImageCustomData(img1)
	c.Check(err, check.IsNil)
	c.Check(imageMetaData.ExposedPort, check.Equals, "3434")
	c.Check(imageMetaData.Processes, check.DeepEquals, map[string][]string{
		"worker1": {"python myapp.py"},
		"worker2": {"someworker"},
	})
}

func (s *S) TestSaveImageCustomDataProcfile(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"exposedPort": "3434",
		"procfile":    "worker1: python myapp.py\nworker2: someworker",
	}
	err := image.SaveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	imageMetaData, err := image.GetImageCustomData(img1)
	c.Check(err, check.IsNil)
	c.Check(imageMetaData.ExposedPort, check.Equals, "3434")
	c.Check(imageMetaData.Processes, check.DeepEquals, map[string][]string{
		"worker1": {"python myapp.py"},
		"worker2": {"someworker"},
	})
}

func (s *S) TestSaveImageCustomDataProcessList(c *check.C) {
	img1 := "tsuru/app-myapp:v1"
	customData1 := map[string]interface{}{
		"exposedPort": "3434",
		"processes": map[string]interface{}{
			"worker1": "python myapp.py",
			"worker2": []string{"worker", "arg", "arg2"},
		},
	}
	err := image.SaveImageCustomData(img1, customData1)
	c.Assert(err, check.IsNil)
	imageMetaData, err := image.GetImageCustomData(img1)
	c.Check(err, check.IsNil)
	c.Check(imageMetaData.ExposedPort, check.Equals, "3434")
	c.Check(imageMetaData.Processes, check.DeepEquals, map[string][]string{
		"worker1": {"python myapp.py"},
		"worker2": {"worker", "arg", "arg2"},
	})
}

func (s *S) TestGetProcessesFromProcfile(c *check.C) {
	tests := []struct {
		procfile string
		expected map[string][]string
	}{
		{procfile: "", expected: map[string][]string{}},
		{procfile: "invalid", expected: map[string][]string{}},
		{procfile: "web: a b c", expected: map[string][]string{
			"web": {"a b c"},
		}},
		{procfile: "web: a b c\nworker: \t  x y z \r  ", expected: map[string][]string{
			"web":    {"a b c"},
			"worker": {"x y z"},
		}},
		{procfile: "web:abc\nworker:xyz", expected: map[string][]string{
			"web":    {"abc"},
			"worker": {"xyz"},
		}},
		{procfile: "web: a b c\r\nworker:x\r\nworker2: z\r\n", expected: map[string][]string{
			"web":     {"a b c"},
			"worker":  {"x"},
			"worker2": {"z"},
		}},
	}
	for i, t := range tests {
		v := image.GetProcessesFromProcfile(t.procfile)
		c.Check(v, check.DeepEquals, t.expected, check.Commentf("failed test %d", i))
	}
}

func (s *S) TestGetImageCustomDataLegacyProcesses(c *check.C) {
	data := image.ImageMetadata{
		Name: "tsuru/app-myapp:v1",
		LegacyProcesses: map[string]string{
			"worker1": "python myapp.py",
			"worker2": "worker2",
		},
	}
	err := data.Save()
	c.Assert(err, check.IsNil)
	dbMetadata, err := image.GetImageCustomData(data.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbMetadata.Processes, check.DeepEquals, map[string][]string{
		"worker1": {"python myapp.py"},
		"worker2": {"worker2"},
	})
	data.Name = "tsuru/app-myapp:v2"
	data.Processes = map[string][]string{
		"w1": {"has", "priority"},
	}
	err = data.Save()
	c.Assert(err, check.IsNil)
	dbMetadata, err = image.GetImageCustomData(data.Name)
	c.Assert(err, check.IsNil)
	c.Assert(dbMetadata.Processes, check.DeepEquals, map[string][]string{
		"w1": {"has", "priority"},
	})
}

func (s *S) TestAllAppProcesses(c *check.C) {
	err := image.AppendAppImageName("myapp", "tsuru/app-myapp:v1")
	c.Assert(err, check.IsNil)
	data := image.ImageMetadata{
		Name: "tsuru/app-myapp:v1",
		Processes: map[string][]string{
			"worker1": {"python myapp.py"},
			"worker2": {"worker2"},
		},
	}
	err = data.Save()
	c.Assert(err, check.IsNil)
	procs, err := image.AllAppProcesses("myapp")
	c.Assert(err, check.IsNil)
	sort.Strings(procs)
	c.Assert(procs, check.DeepEquals, []string{"worker1", "worker2"})
}
