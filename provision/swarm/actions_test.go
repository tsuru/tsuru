// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"net/http"

	"github.com/docker/docker/api/types/swarm"
	"github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/tsuru/tsuru/action"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/check.v1"
)

func (s *S) TestActionUpdateServicesForward(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "app:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	args := &pipelineArgs{
		client:   cli,
		app:      a,
		newImage: imgName,
		newImgData: &image.ImageMetadata{
			Processes: map[string]string{"web": ""},
		},
		currentImgData: &image.ImageMetadata{},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, []string{"web"})
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
}

func (s *S) TestActionUpdateServicesForwardUpdateExisting(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Command: []string{"oldcmd"},
				},
			},
			Annotations: swarm.Annotations{
				Name: "myapp-web",
			},
		},
	})
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "app:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	args := &pipelineArgs{
		client:   cli,
		app:      a,
		newImage: imgName,
		newImgData: &image.ImageMetadata{
			Processes: map[string]string{"web": ""},
		},
		currentImgData: &image.ImageMetadata{},
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, []string{"web"})
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
}

func (s *S) TestActionUpdateServicesForwardFailureInMiddle(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Command: []string{"original-web"},
				},
			},
			Annotations: swarm.Annotations{
				Name: "myapp-web",
			},
		},
	})
	c.Assert(err, check.IsNil)
	srvWorker, err := cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Command: []string{"original-worker"},
				},
			},
			Annotations: swarm.Annotations{
				Name: "myapp-worker",
			},
		},
	})
	c.Assert(err, check.IsNil)
	oldImg := "app:v1"
	err = image.SaveImageCustomData(oldImg, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "old1",
			"worker": "old2",
		},
	})
	c.Assert(err, check.IsNil)
	newImg := "app:v2"
	err = image.SaveImageCustomData(newImg, map[string]interface{}{
		"processes": map[string]interface{}{
			"web":    "new1",
			"worker": "new2",
		},
	})
	c.Assert(err, check.IsNil)
	srv.CustomHandler("/services/"+srvWorker.ID, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("bad error"))
	}))
	imgData := &image.ImageMetadata{
		Processes: map[string]string{"web": "", "worker": ""},
	}
	args := &pipelineArgs{
		client:         cli,
		app:            a,
		newImage:       newImg,
		newImgData:     imgData,
		currentImage:   oldImg,
		currentImgData: imgData,
	}
	processes, err := updateServices.Forward(action.FWContext{Params: []interface{}{args}})
	c.Assert(err, check.ErrorMatches, ".*bad error")
	c.Assert(processes, check.IsNil)
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec old1",
	})
	service, err = cli.InspectService("myapp-worker")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{
		"original-worker",
	})
}

func (s *S) TestActionUpdateServicesBackward(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	imgName := "app:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	args := &pipelineArgs{
		client:       cli,
		app:          a,
		currentImage: imgName,
		newImgData: &image.ImageMetadata{
			Processes: map[string]string{"web": ""},
		},
		currentImgData: &image.ImageMetadata{
			Processes: map[string]string{"web": ""},
		},
	}
	updateServices.Backward(action.BWContext{
		FWResult: []string{"web"},
		Params:   []interface{}{args},
	})
	service, err := cli.InspectService("myapp-web")
	c.Assert(err, check.IsNil)
	c.Assert(service.Spec.TaskTemplate.ContainerSpec.Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec python myapp.py",
	})
}

func (s *S) TestActionUpdateServicesBackwardNotInCurrent(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	metadata := map[string]string{"m1": "v1", "pool": "p1"}
	opts := provision.AddNodeOptions{
		Address:  srv.URL(),
		Metadata: metadata,
	}
	err = s.p.AddNode(opts)
	c.Assert(err, check.IsNil)
	cli, err := chooseDBSwarmNode()
	c.Assert(err, check.IsNil)
	a := &app.App{Name: "myapp", Platform: "whitespace", TeamOwner: s.team.Name}
	err = app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	_, err = cli.CreateService(docker.CreateServiceOptions{
		ServiceSpec: swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Command: []string{"original-web"},
				},
			},
			Annotations: swarm.Annotations{
				Name: "myapp-web",
			},
		},
	})
	c.Assert(err, check.IsNil)
	imgName := "app:v1"
	err = image.SaveImageCustomData(imgName, map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	args := &pipelineArgs{
		client:       cli,
		app:          a,
		currentImage: imgName,
		newImgData: &image.ImageMetadata{
			Processes: map[string]string{"web": ""},
		},
		currentImgData: &image.ImageMetadata{},
	}
	updateServices.Backward(action.BWContext{
		FWResult: []string{"web"},
		Params:   []interface{}{args},
	})
	_, err = cli.InspectService("myapp-web")
	c.Assert(err, check.ErrorMatches, "No such service.*")
}
