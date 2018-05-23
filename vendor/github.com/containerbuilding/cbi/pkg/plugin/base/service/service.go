/*
Copyright The CBI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	api "github.com/containerbuilding/cbi/pkg/plugin/api"
	"github.com/containerbuilding/cbi/pkg/plugin/base"
)

type Service struct {
	Backend base.Backend
}

func ServeTCP(s *Service, port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	glog.Infof("Using address %q", ln.Addr().String())
	gs := grpc.NewServer()
	api.RegisterPluginServer(gs, s)
	return gs.Serve(ln)
}

func (s *Service) Info(ctx context.Context, req *api.InfoRequest) (*api.InfoResponse, error) {
	return s.Backend.Info(ctx, req)
}

func (s *Service) Spec(ctx context.Context, req *api.SpecRequest) (*api.SpecResponse, error) {
	var buildJob crd.BuildJob
	if err := json.Unmarshal(req.BuildJobJson, &buildJob); err != nil {
		return nil, err
	}
	sp, err := s.Backend.CreatePodTemplateSpec(ctx, buildJob)
	if err != nil {
		return nil, err
	}
	spJSON, err := json.Marshal(sp)
	if err != nil {
		return nil, err
	}
	res := &api.SpecResponse{
		PodTemplateSpecJson: spJSON,
	}
	return res, nil
}
