// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	buildpb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	check "gopkg.in/check.v1"

	"github.com/tsuru/tsuru/app/bind"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	imagetypes "github.com/tsuru/tsuru/types/app/image"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

const buildServiceAddressKey = "build-service-address"

func (s *S) TestBuildV2_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.b.BuildV2(ctx, nil, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "context canceled")
}

func (s *S) TestBuildV2_MissingApp(c *check.C) {
	_, err := s.b.BuildV2(context.TODO(), nil, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "app not provided")
}

func (s *S) TestBuildV2_MissingEvent(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	_, err := s.b.BuildV2(context.TODO(), a, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event not provided")
}

func (s *S) TestBuildV2_MissingBuildServiceAddress(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	_, err = s.b.BuildV2(context.TODO(), a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "build service address not provided: deploy v2 not supported")
	c.Assert(errors.Is(err, provision.ErrDeployV2NotSupported), check.Equals, true)
}

func (s *S) TestBuildV2_BuildServiceReturnsError(c *check.C) {
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

	_, err = s.b.BuildV2(context.TODO(), a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Unknown, "some error has been occurred").Error())
}

func (s *S) TestBuildV2_BuildServiceShouldRespectContextCancelation(c *check.C) {
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

	_, err = s.b.BuildV2(ctx, a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Canceled, "context canceled").Error())
}

func (s *S) TestBuildV2_BuildWithSourceCode(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
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
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_SOURCE_UPLOAD)
			c.Check(req.GetSourceImage(), check.DeepEquals, "docker.io/tsuru/python:latest")
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:latest"})
			c.Check(req.GetData(), check.DeepEquals, []byte(`my awesome source code :P`))

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

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	data := bytes.NewBufferString("my awesome source code :P")

	var output bytes.Buffer

	appVersion, err := s.b.BuildV2(context.TODO(), a, evt, builder.BuildOpts{
		Message:     "Add my awesome feature :P",
		ArchiveFile: data,
		ArchiveSize: int64(data.Len()),
		Output:      &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(output.String(), check.Matches, "(?s).*--> Starting container image build(.*)")
	c.Assert(output.String(), check.Matches, "(?s).*------- some container build progress(.*)")
	c.Assert(output.String(), check.Matches, "(?s).*--> Container image build finished(.*)")

	c.Assert(appVersion, check.NotNil)
	c.Assert(appVersion.Version(), check.DeepEquals, 1)
	c.Assert(appVersion.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(appVersion.VersionInfo().Description, check.DeepEquals, "Add my awesome feature :P")
	c.Assert(appVersion.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")

	processes, err := appVersion.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web":    {"./path/app.py --port ${PORT}"},
		"worker": {"./path/worker.sh --verbose"},
	})

	tsuruYaml, err := appVersion.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{
		Healthcheck: &provisiontypes.TsuruYamlHealthcheck{
			Path: "/healthz",
		},
		Hooks: &provisiontypes.TsuruYamlHooks{
			Build: []string{"mkdir /path/to/my/dir", "/path/to/script.sh"},
		},
	})
}

func (s *S) TestBuildV2_BuildWithContainerImage(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetApp(), check.DeepEquals, &buildpb.TsuruApp{Name: "myapp"})
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_CONTAINER_IMAGE)
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

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	var output bytes.Buffer

	appVersion, err := s.b.BuildV2(context.TODO(), a, evt, builder.BuildOpts{
		Message: "New container image xD",
		ImageID: "registry.example/my-repository/my-app:v42",
		Output:  &output,
	})
	c.Assert(err, check.IsNil)

	c.Assert(appVersion.Version(), check.DeepEquals, 1)
	c.Assert(appVersion.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(appVersion.VersionInfo().Description, check.DeepEquals, "New container image xD")
	c.Assert(appVersion.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")
	c.Assert(appVersion.VersionInfo().ExposedPorts, check.DeepEquals, []string{"8080/tcp", "8443/tcp"})

	processes, err := appVersion.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web": {"/bin/sh", "-c", "/var/www/app/app.sh", "--port", "${PORT}"},
	})

	tsuruYaml, err := appVersion.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{})
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
