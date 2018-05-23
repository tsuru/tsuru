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

package pluginselector

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	api "github.com/containerbuilding/cbi/pkg/plugin/api"
)

type PluginSelectorFunc func(plugins []api.InfoResponse, bj crd.BuildJob) (int, error)

// FIXME: we really should have grpc.ClientConn here.
func NewPluginSelector(fn PluginSelectorFunc, conns ...*grpc.ClientConn) *PluginSelector {
	ps := &PluginSelector{
		fn:         fn,
		cachedInfo: make(map[*grpc.ClientConn]*api.InfoResponse, len(conns)),
	}
	for _, conn := range conns {
		ps.cachedInfo[conn] = nil
	}
	return ps
}

type PluginSelector struct {
	fn         PluginSelectorFunc
	cachedInfo map[*grpc.ClientConn]*api.InfoResponse
}

func (ps *PluginSelector) UpdateCachedInfo(ctx context.Context) error {
	var errors []error
	for conn := range ps.cachedInfo {
		client := api.NewPluginClient(conn)
		info, err := client.Info(ctx, &api.InfoRequest{})
		if err != nil {
			errors = append(errors, err)
			info = nil
		}
		ps.cachedInfo[conn] = info
	}
	if len(errors) > 0 {
		return fmt.Errorf("%v", errors)
	}
	return nil
}

func (ps *PluginSelector) Select(bj crd.BuildJob) api.PluginClient {
	var (
		conns []*grpc.ClientConn
		info  []api.InfoResponse
	)

	for c, i := range ps.cachedInfo {
		if i != nil {
			conns = append(conns, c)
			info = append(info, *i)
		}
	}
	idx, err := ps.fn(info, bj)
	if err != nil {
		glog.Warning(err)
	}
	if idx >= 0 {
		conn := conns[idx]
		return api.NewPluginClient(conn)
	}
	return nil
}
