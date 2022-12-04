// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"testing"
	"time"

	buildpb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	check "gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/servicemanager"
	imagetypes "github.com/tsuru/tsuru/types/app/image"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

func (s *S) TestDeployV2_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.p.DeployV2(ctx, nil, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "context canceled")
}

func (s *S) TestDeployV2_MissingApp(c *check.C) {
	_, err := s.p.DeployV2(context.TODO(), nil, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "app not provided")
}

func (s *S) TestDeployV2_MissingEvent(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	_, err := s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event not provided")
}

func (s *S) TestDeployV2_Unsupported(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			return status.Errorf(codes.Unimplemented, "build not supported for the provided deploy origin")
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	_, err = s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{Event: evt})
	c.Assert(err, check.NotNil)
	c.Assert(errors.Is(err, provision.ErrDeployV2NotSupported), check.Equals, true)
}

func (s *S) TestDeployV2_BuildServiceReturnsError(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			return fmt.Errorf("some error has been occurred")
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	_, err = s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{Event: evt})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Unknown, "some error has been occurred").Error())
}

func (s *S) TestDeployV2_BuildServiceShouldRespectContextCancelation(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	ch := make(chan struct{})
	defer close(ch)

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			ch <- struct{}{}
			time.Sleep(time.Second)
			return fmt.Errorf("should not pass here")
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-ch
		cancel()
	}()

	_, err = s.p.DeployV2(ctx, a, provision.DeployV2Args{Event: evt})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Canceled, "context canceled").Error())
}

func (s *S) TestDeployV2_DeployFromSourceCode(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	a.SetEnv(bind.EnvVar{Name: "MY_ENV1", Value: "value 1"})
	a.SetEnv(bind.EnvVar{Name: "MY_ENV2", Value: "value 2"})

	s.mockService.PlatformImage.OnCurrentImage = func(registry imagetypes.ImageRegistry, platform string) (string, error) {
		if c.Check(registry, check.DeepEquals, imagetypes.ImageRegistry("")) &&
			c.Check(platform, check.DeepEquals, "python") {
			return "docker.io/tsuru/python:latest", nil
		}

		return "", fmt.Errorf("bad platform")
	}

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetApp(), check.DeepEquals, &buildpb.TsuruApp{
				Name: "myapp",
				EnvVars: map[string]string{
					"MY_ENV1": "value 1",
					"MY_ENV2": "value 2",
				},
			})
			c.Check(req.GetDeployOrigin(), check.DeepEquals, buildpb.DeployOrigin_DEPLOY_ORIGIN_SOURCE_FILES)
			c.Check(req.GetSourceImage(), check.DeepEquals, "docker.io/tsuru/python:latest")
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:latest"})
			c.Check(req.GetData(), check.DeepEquals, []byte(`my awesome source code :P`))
			c.Check(req.GetPushOptions(), check.DeepEquals, &buildpb.PushOptions{})

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "--> Starting container image build\n"}})
			c.Check(err, check.IsNil)

			err = stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "------- some container build progress\n"}})
			c.Check(err, check.IsNil)

			err = stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "--> Container image build finished\n"}})
			c.Check(err, check.IsNil)

			err = stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{
				Procfile: "web: ./path/app.py --port ${PORT}\nworker: ./path/worker.sh --verbose",
				TsuruYaml: `
healthcheck:
  path: /healthz

hooks:
  build:
  - mkdir /path/to/my/dir
  - /path/to/script.sh
`,
			}}})
			c.Assert(err, check.IsNil)

			return nil
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[disableUnitRegisterCmdKey] = "true"

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	data := bytes.NewBufferString("my awesome source code :P")

	var output bytes.Buffer

	image, err := s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{
		Description: "Add my awesome feature :P",
		Kind:        string(app.DeployUpload),
		Event:       evt,
		Output:      &output,
		Archive:     io.NopCloser(data),
		ArchiveSize: int64(data.Len()),
	})
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "tsuru/app-myapp:v1")
	c.Assert(output.String(), check.Matches, "(?s).*--> Starting container image build(.*)")
	c.Assert(output.String(), check.Matches, "(?s).*------- some container build progress(.*)")
	c.Assert(output.String(), check.Matches, "(?s).*--> Container image build finished(.*)")

	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.DeepEquals, 1)
	c.Assert(version.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(version.VersionInfo().Description, check.DeepEquals, "Add my awesome feature :P")
	c.Assert(version.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")

	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web":    []string{"./path/app.py --port ${PORT}"},
		"worker": []string{"./path/worker.sh --verbose"},
	})

	tsuruYaml, err := version.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{
		Healthcheck: &provisiontypes.TsuruYamlHealthcheck{
			Path: "/healthz",
		},
		Hooks: &provisiontypes.TsuruYamlHooks{
			Build: []string{"mkdir /path/to/my/dir", "/path/to/script.sh"},
		},
	})

	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 2)

	sort.Slice(deps.Items, func(i, j int) bool { return deps.Items[i].Name < deps.Items[j].Name })

	webDeployment := deps.Items[0]
	c.Assert(webDeployment.Name, check.Equals, "myapp-web")
	c.Assert(webDeployment.Spec.Template.Spec.Containers, check.HasLen, 1)
	c.Assert(webDeployment.Spec.Template.Spec.Containers[0].Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec ./path/app.py --port ${PORT}",
	})

	workerDeployment := deps.Items[1]
	c.Assert(workerDeployment.Name, check.Equals, "myapp-worker")
	c.Assert(workerDeployment.Spec.Template.Spec.Containers, check.HasLen, 1)
	c.Assert(workerDeployment.Spec.Template.Spec.Containers[0].Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec ./path/worker.sh --verbose",
	})

	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 2)

	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(appList.Items, check.HasLen, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:        ns,
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}, "worker": {"myapp-worker"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}, "worker": {"myapp-worker", "myapp-worker-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}, "worker": {"myapp-worker"}},
	})
}

func (s *S) TestDeployV2_DeployFromContainerImage(c *check.C) {
	a, wait, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetApp(), check.DeepEquals, &buildpb.TsuruApp{Name: "myapp"})
			c.Check(req.GetDeployOrigin(), check.DeepEquals, buildpb.DeployOrigin_DEPLOY_ORIGIN_CONTAINER_IMAGE)
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:latest"})
			c.Check(req.GetData(), check.IsNil)

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{
				ImageConfig: &buildpb.ContainerImageConfig{
					Entrypoint:   []string{"/bin/sh", "-c"},
					Cmd:          []string{"/var/www/app/app.sh", "--port", "${PORT}"},
					ExposedPorts: []string{"8080/tcp", "8443/tcp"},
					WorkingDir:   "/var/www/app",
				},
			}}})
			c.Check(err, check.IsNil)

			return nil
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[disableUnitRegisterCmdKey] = "true"

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	var output bytes.Buffer

	image, err := s.p.DeployV2(context.TODO(), a, provision.DeployV2Args{
		Description: "New container image xD",
		Kind:        string(app.DeployImage),
		Image:       "registry.example/my-repository/my-app:v42",
		Event:       evt,
		Output:      &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(image, check.Equals, "tsuru/app-myapp:v1")

	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(version.Version(), check.DeepEquals, 1)
	c.Assert(version.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(version.VersionInfo().Description, check.DeepEquals, "New container image xD")
	c.Assert(version.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")
	c.Assert(version.VersionInfo().ExposedPorts, check.DeepEquals, []string{"8080/tcp", "8443/tcp"})

	processes, err := version.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web": []string{"/bin/sh", "-c", "/var/www/app/app.sh", "--port", "${PORT}"},
	})

	tsuruYaml, err := version.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{})

	wait()

	ns, err := s.client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)

	deps, err := s.client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(deps.Items, check.HasLen, 1)

	webDeployment := deps.Items[0]
	c.Assert(webDeployment.Name, check.Equals, "myapp-web")
	c.Assert(webDeployment.Spec.Template.Spec.Containers, check.HasLen, 1)

	c.Assert(webDeployment.Spec.Template.Spec.Containers[0].Command, check.DeepEquals, []string{
		"/bin/sh", "-lc", "[ -d /home/application/current ] && cd /home/application/current; exec $0 \"$@\"",
		"/bin/sh", "-c", "/var/www/app/app.sh", "--port", "${PORT}",
	})

	units, err := s.p.Units(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(units, check.HasLen, 1)

	svcs, err := s.client.CoreV1().Services(ns).List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(svcs.Items, check.HasLen, 2)

	sort.Slice(svcs.Items, func(i, j int) bool { return svcs.Items[i].Name < svcs.Items[j].Name })

	webSvc := svcs.Items[0]
	c.Assert(webSvc.Name, check.DeepEquals, "myapp-web")

	webUnitsSvc := svcs.Items[1]
	c.Assert(webUnitsSvc.Name, check.DeepEquals, "myapp-web-units")

	appList, err := s.client.TsuruV1().Apps("tsuru").List(context.TODO(), metav1.ListOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(appList.Items, check.HasLen, 1)
	c.Assert(appList.Items[0].Spec, check.DeepEquals, tsuruv1.AppSpec{
		NamespaceName:        ns,
		ServiceAccountName:   "app-myapp",
		Deployments:          map[string][]string{"web": {"myapp-web"}},
		Services:             map[string][]string{"web": {"myapp-web", "myapp-web-units"}},
		PodDisruptionBudgets: map[string][]string{"web": {"myapp-web"}},
	})
}

func setupBuildServer(t *testing.T, bs buildpb.BuildServer) string {
	t.Helper()

	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("failed to create temporary listener: %v", err)
	}

	s := grpc.NewServer()
	t.Cleanup(func() { s.Stop() })

	buildpb.RegisterBuildServer(s, bs)

	go func() {
		if err := s.Serve(l); err != nil {
			t.Errorf("build server finished with error: %v", err)
		}
	}()

	return l.Addr().String()
}

type fakeBuildServer struct {
	*buildpb.UnimplementedBuildServer

	OnBuild func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error
}

func (f *fakeBuildServer) Build(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
	if f.OnBuild == nil {
		return fmt.Errorf("fake method not implemented")
	}

	return f.OnBuild(req, stream)
}
