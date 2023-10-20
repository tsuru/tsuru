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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	buildpb "github.com/tsuru/deploy-agent/pkg/build/grpc_build_v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	check "gopkg.in/check.v1"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	appTypes "github.com/tsuru/tsuru/types/app"
	imagetypes "github.com/tsuru/tsuru/types/app/image"
	bindTypes "github.com/tsuru/tsuru/types/bind"
	"github.com/tsuru/tsuru/types/job"
	provisiontypes "github.com/tsuru/tsuru/types/provision"
)

const (
	buildServiceAddressKey  = "build-service-address"
	registryKey             = "registry"
	registryInsecureKey     = "registry-insecure"
	disablePlatformBuildKey = "disable-platform-build"
)

func (s *S) TestBuild_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.b.Build(ctx, nil, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "context canceled")
}

func (s *S) TestBuild_MissingApp(c *check.C) {
	_, err := s.b.Build(context.TODO(), nil, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "app not provided")
}

func (s *S) TestBuildJob_MissingJob(c *check.C) {
	_, err := s.b.BuildJob(context.TODO(), nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "job not provided")
}

func (s *S) TestBuild_MissingEvent(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	_, err := s.b.Build(context.TODO(), a, nil, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "event not provided")
}

func (s *S) TestBuild_BuildFromRebuild(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{Rebuild: true})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "app rebuild is deprecated")
}

func (s *S) TestBuildJob_MissingBuildServiceAddress(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	_, err := s.b.BuildJob(context.TODO(), &job.Job{Name: "test"}, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "build service address not provided: build v2 not supported")
	c.Assert(errors.Is(err, builder.ErrBuildV2NotSupported), check.Equals, true)
}

func (s *S) TestBuild_MissingBuildServiceAddress(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "build service address not provided: build v2 not supported")
	c.Assert(errors.Is(err, builder.ErrBuildV2NotSupported), check.Equals, true)
}

func (s *S) TestBuild_BuildServiceReturnsError(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			return fmt.Errorf("some error occurred")
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

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Unknown, "some error occurred").Error())
}

func (s *S) TestBuildJob_BuildServiceReturnsError(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			return fmt.Errorf("some error occurred")
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	_, err := s.b.BuildJob(context.TODO(), &job.Job{}, builder.BuildOpts{ImageID: "my-image"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Unknown, "some error occurred").Error())
}

func (s *S) TestBuild_BuildServiceShouldRespectContextCancelation(c *check.C) {
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

	_, err = s.b.Build(ctx, a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Canceled, "context canceled").Error())
}

func (s *S) TestBuildJob_BuildServiceShouldRespectContextCancelation(c *check.C) {
	_, _, rollback := s.mock.DefaultReactions(c)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-ch
		cancel()
	}()

	_, err := s.b.BuildJob(ctx, &job.Job{}, builder.BuildOpts{ImageID: "my-image"})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Canceled, "context canceled").Error())
}

func (s *S) TestBuild_BuildWithSourceCode(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	a.SetEnv(bindTypes.EnvVar{Name: "MY_ENV1", Value: "value 1"})
	a.SetEnv(bindTypes.EnvVar{Name: "MY_ENV2", Value: "value 2"})

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

kubernetes:
  groups:
    my-app:
      web:
        ports:
        - name: http
          port: 80
          target_port: 8080
        - name: http-grpc
          port: 3000
          protocol: TCP
`,
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

	data := bytes.NewBufferString("my awesome source code :P")

	var output bytes.Buffer

	appVersion, err := s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
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
		Kubernetes: &provisiontypes.TsuruYamlKubernetesConfig{
			Groups: map[string]provisiontypes.TsuruYamlKubernetesGroup{
				"my-app": map[string]provisiontypes.TsuruYamlKubernetesProcessConfig{
					"web": {
						Ports: []provisiontypes.TsuruYamlKubernetesProcessPortConfig{
							{Name: "http", Port: 80, TargetPort: 8080},
							{Name: "http-grpc", Port: 3000, Protocol: "TCP"},
						},
					},
				},
			},
		},
	})
}

func (s *S) TestBuild_BuildWithContainerImage(c *check.C) {
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

	appVersion, err := s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
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

func (s *S) TestBuildJob_BuildWithContainerImage(c *check.C) {
	s.mockService.JobService.OnBaseImageName = func(ctx context.Context, j *job.Job) (string, error) {
		reg := imagetypes.ImageRegistry("tsuru.io")
		newImage, err := image.JobBasicImageName(reg, j.Name)
		c.Assert(err, check.IsNil)
		return fmt.Sprintf("%s:latest", newImage), nil
	}
	_, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetJob(), check.DeepEquals, &buildpb.TsuruJob{Name: "myjob"})
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_JOB_CREATE_WITH_CONTAINER_IMAGE)
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru.io/job-myjob:latest"})
			c.Check(req.GetData(), check.IsNil)

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{}}})
			c.Check(err, check.IsNil)

			return nil
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	var output bytes.Buffer

	buildJob := &job.Job{
		Name: "myjob",
		Spec: job.JobSpec{
			Container: job.ContainerInfo{
				Image:   "my-job:v1",
				Command: []string{"echo", "hello world"},
			},
		},
	}
	newImageDst, err := s.b.BuildJob(context.TODO(), buildJob, builder.BuildOpts{
		Message: "New container image xD",
		ImageID: "my-job:v1",
		Output:  &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(newImageDst, check.Equals, "tsuru.io/job-myjob:latest")
}

func (s *S) TestBuild_DeployWithContainerImage_NoImageConfigReturned(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{}}}) // tsuru config w/ image config not set
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

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "neither Procfile nor entrypoint and cmd set")
}

func (s *S) TestBuild_BuildWithArchiveURL_FailedToDownloadArchive(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, check.Equals, "GET")
		c.Check(r.URL.Path, check.Equals, "/my-app/code.tar.gz")

		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "missing authentication")
	}))
	defer srv.Close()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	var output bytes.Buffer

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
		ArchiveURL: srv.URL + "/my-app/code.tar.gz",
		Output:     &output,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "could not download the archive: unexpected status code")
}

func (s *S) TestBuild_BuildWithArchiveURL_MissingArchive(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, check.Equals, "GET")
		c.Check(r.URL.Path, check.Equals, "/my-app/code.tar.gz")

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	evt, err := event.New(&event.Opts{
		Target:  event.Target{Type: event.TargetTypeApp, Value: a.GetName()},
		Kind:    permission.PermAppDeploy,
		Owner:   s.token,
		Allowed: event.Allowed(permission.PermAppDeploy),
	})
	c.Assert(err, check.IsNil)

	var output bytes.Buffer

	_, err = s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
		ArchiveURL: srv.URL + "/my-app/code.tar.gz",
		Output:     &output,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "archive file is empty")
}

func (s *S) TestBuild_BuildWithArchiveURL(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, check.Equals, "GET")
		c.Check(r.URL.Path, check.Equals, "/my-app/code.tar.gz")

		fmt.Fprintf(w, "awesome tarball with source code")
	}))
	defer srv.Close()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetApp(), check.DeepEquals, &buildpb.TsuruApp{Name: "myapp"})
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_SOURCE_UPLOAD)
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:latest"})
			c.Check(req.GetData(), check.DeepEquals, []byte(`awesome tarball with source code`))

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{
				Procfile: "web: /path/to/my/server.sh --port ${PORT}",
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

	appVersion, err := s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
		Message:    "My deploy with archive URL",
		ArchiveURL: srv.URL + "/my-app/code.tar.gz",
		Output:     &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(appVersion.Version(), check.Equals, 1)
	c.Assert(appVersion.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(appVersion.VersionInfo().Description, check.DeepEquals, "My deploy with archive URL")
	c.Assert(appVersion.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")

	processes, err := appVersion.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web": {"/path/to/my/server.sh --port ${PORT}"},
	})

	tsuruYaml, err := appVersion.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{})
}

func (s *S) TestBuild_BuildWithDockerfile(c *check.C) {
	a, _, rollback := s.mock.DefaultReactions(c)
	defer rollback()

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			// NOTE(nettoclaudio): cannot call c.Assert here since it might call runtime.Goexit and
			// provoke an deadlock on RPC client and server.
			c.Check(req.GetApp(), check.DeepEquals, &buildpb.TsuruApp{Name: "myapp"})
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_APP_BUILD_WITH_CONTAINER_FILE)
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"tsuru/app-myapp:v1", "tsuru/app-myapp:latest"})
			c.Check(req.GetContainerfile(), check.Equals, `FROM alpine:latest
RUN set -x \
    apk add curl tar make
COPY ./tsuru.yaml ./app.sh /var/app/
WORKDIR /var/app
EXPOSE 8888/tcp
ENTRYPOINT ["/var/app/app.sh"]
CMD ["--port", "8888"]
`)
			c.Check(req.GetData(), check.DeepEquals, []byte("some context files"))

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_TsuruConfig{TsuruConfig: &buildpb.TsuruConfig{
				TsuruYaml: "healthcheck:\n  path: /healthz",
				ImageConfig: &buildpb.ContainerImageConfig{
					Entrypoint:   []string{"/var/app/app.sh"},
					Cmd:          []string{"--port", "8888"},
					ExposedPorts: []string{"8888/tcp"},
					WorkingDir:   "/var/app",
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

	appVersion, err := s.b.Build(context.TODO(), a, evt, builder.BuildOpts{
		Message:     "My deploy w/ Dockerfile",
		ArchiveFile: strings.NewReader("some context files"),
		ArchiveSize: 18,
		Dockerfile: `FROM alpine:latest
RUN set -x \
    apk add curl tar make
COPY ./tsuru.yaml ./app.sh /var/app/
WORKDIR /var/app
EXPOSE 8888/tcp
ENTRYPOINT ["/var/app/app.sh"]
CMD ["--port", "8888"]
`,
	})
	c.Assert(err, check.IsNil)

	c.Assert(appVersion.Version(), check.DeepEquals, 1)
	c.Assert(appVersion.VersionInfo().EventID, check.DeepEquals, evt.UniqueID.Hex())
	c.Assert(appVersion.VersionInfo().Description, check.DeepEquals, "My deploy w/ Dockerfile")
	c.Assert(appVersion.VersionInfo().DeployImage, check.DeepEquals, "tsuru/app-myapp:v1")

	processes, err := appVersion.Processes()
	c.Assert(err, check.IsNil)
	c.Assert(processes, check.DeepEquals, map[string][]string{
		"web": {"/var/app/app.sh", "--port", "8888"},
	})

	tsuruYaml, err := appVersion.TsuruYamlData()
	c.Assert(err, check.IsNil)
	c.Assert(tsuruYaml, check.DeepEquals, provisiontypes.TsuruYamlData{
		Healthcheck: &provisiontypes.TsuruYamlHealthcheck{
			Path: "/healthz",
		},
	})
}

func (s *S) TestPlatformBuild_ContextCanceled(c *check.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.b.PlatformBuild(ctx, appTypes.PlatformOptions{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "context canceled")
}

func (s *S) TestPlatformBuild_MissingBuildServiceAddress(c *check.C) {
	_, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "build service address not provided: build v2 not supported")
	c.Assert(errors.Is(err, builder.ErrBuildV2NotSupported), check.Equals, true)
}

func (s *S) TestPlatformBuild_ClusterWithPlatformBuildDisabled(c *check.C) {
	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[disablePlatformBuildKey] = "true"

	var output bytes.Buffer
	_, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{
		Output: &output,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "no kubernetes nodes available")
	c.Assert(output.String(), check.Matches, "(?s).*Skipping platform build on c1 cluster: disabled to platform builds.*")
}

func (s *S) TestPlatformBuild_ClusterWithoutRegistry(c *check.C) {
	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress

	var output bytes.Buffer
	_, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{
		Output: &output,
	})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "no kubernetes nodes available")
	c.Assert(output.String(), check.Matches, "(?s).*Skipping platform build on c1 cluster: no registry found in cluster configs.*")
}

func (s *S) TestPlatformBuild_RollbackVersion(c *check.C) {
	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[registryKey] = "registry.example.com/tsuru"

	_, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{RollbackVersion: 42})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "rollback not implemented")
}

func (s *S) TestPlatformBuild_BuildServiceReturnsError(c *check.C) {
	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			return errors.New("something went wrong")
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[registryKey] = "registry.example.com/tsuru"

	_, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{})
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, status.Errorf(codes.Unknown, "something went wrong").Error())
}

func (s *S) TestPlatformBuild_SuccesfulPlatformBuild(c *check.C) {
	s.mockService.PlatformImage.OnNewImage = func(reg imagetypes.ImageRegistry, platform string, version int) (string, error) {
		c.Check(reg, check.DeepEquals, imagetypes.ImageRegistry("registry.example.com/tsuru"))
		c.Check(platform, check.DeepEquals, "my-platform")
		c.Check(version, check.Equals, 42)

		return "registry.example.com/tsuru/my-platform:v42", nil
	}

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_PLATFORM_WITH_CONTAINER_FILE)
			c.Check(req.GetPlatform(), check.DeepEquals, &buildpb.TsuruPlatform{Name: "my-platform"})
			c.Check(req.GetContainerfile(), check.DeepEquals, "FROM tsuru/scratch:latest")
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"registry.example.com/tsuru/my-platform:v42", "registry.example.com/tsuru/my-platform:latest"})
			c.Check(req.GetPushOptions(), check.DeepEquals, &buildpb.PushOptions{InsecureRegistry: false})

			err := stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "--> Starting container image build\n"}})
			c.Check(err, check.IsNil)

			err = stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "SOME OUTPUT FROM BUILD SERVICE\n"}})
			c.Check(err, check.IsNil)

			err = stream.Send(&buildpb.BuildResponse{Data: &buildpb.BuildResponse_Output{Output: "--> Container image build finished\n"}})
			c.Check(err, check.IsNil)

			return nil
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[registryKey] = "registry.example.com/tsuru"

	var output bytes.Buffer
	images, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{
		Name:      "my-platform",
		Version:   42,
		ExtraTags: []string{"latest"},
		Data:      []byte(`FROM tsuru/scratch:latest`),
		Output:    &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"registry.example.com/tsuru/my-platform:v42", "registry.example.com/tsuru/my-platform:latest"})
	c.Assert(output.String(), check.Matches, "(?s).*---- Building platform my-platform on cluster c1 ----.*")
	c.Assert(output.String(), check.Matches, "(?s).* ---> Destination image: registry.example.com/tsuru/my-platform:v42.*")
	c.Assert(output.String(), check.Matches, "(?s).* ---> Destination image: registry.example.com/tsuru/my-platform:latest.*")
	c.Assert(output.String(), check.Matches, "(?s).*---- Starting build ----.*")
	c.Assert(output.String(), check.Matches, "(?s).*--> Starting container image build.*")
	c.Assert(output.String(), check.Matches, "(?s).*SOME OUTPUT FROM BUILD SERVICE.*")
	c.Assert(output.String(), check.Matches, "(?s).*--> Container image build finished.*")
}

func (s *S) TestPlatformBuild_InsecureRegistry_SuccessfulPlatformBuild(c *check.C) {
	s.mockService.PlatformImage.OnNewImage = func(reg imagetypes.ImageRegistry, platform string, version int) (string, error) {
		c.Check(reg, check.DeepEquals, imagetypes.ImageRegistry("registry.example.com/tsuru"))
		c.Check(platform, check.DeepEquals, "my-platform")
		c.Check(version, check.Equals, 42)

		return "registry.example.com/tsuru/my-platform:v42", nil
	}

	buildServiceAddress := setupBuildServer(s.t, &fakeBuildServer{
		OnBuild: func(req *buildpb.BuildRequest, stream buildpb.Build_BuildServer) error {
			c.Check(req.GetKind(), check.DeepEquals, buildpb.BuildKind_BUILD_KIND_PLATFORM_WITH_CONTAINER_FILE)
			c.Check(req.GetPlatform(), check.DeepEquals, &buildpb.TsuruPlatform{Name: "my-platform"})
			c.Check(req.GetContainerfile(), check.DeepEquals, "FROM tsuru/scratch:latest")
			c.Check(req.GetDestinationImages(), check.DeepEquals, []string{"registry.example.com/tsuru/my-platform:v42", "registry.example.com/tsuru/my-platform:latest"})
			c.Check(req.GetPushOptions(), check.DeepEquals, &buildpb.PushOptions{InsecureRegistry: true})
			return nil
		},
	})
	s.clusterClient.CustomData[buildServiceAddressKey] = buildServiceAddress
	s.clusterClient.CustomData[registryKey] = "registry.example.com/tsuru"
	s.clusterClient.CustomData[registryInsecureKey] = "true"

	var output bytes.Buffer
	images, err := s.b.PlatformBuild(context.TODO(), appTypes.PlatformOptions{
		Name:      "my-platform",
		Version:   42,
		ExtraTags: []string{"latest"},
		Data:      []byte(`FROM tsuru/scratch:latest`),
		Output:    &output,
	})
	c.Assert(err, check.IsNil)
	c.Assert(images, check.DeepEquals, []string{"registry.example.com/tsuru/my-platform:v42", "registry.example.com/tsuru/my-platform:latest"})
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
