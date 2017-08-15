// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/healer"
	"github.com/tsuru/tsuru/iaas"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"gopkg.in/mgo.v2"
)

func validateNodeAddress(address string) error {
	if address == "" {
		return errors.Errorf("address=url parameter is required")
	}
	url, err := url.ParseRequestURI(address)
	if err != nil {
		return errors.Wrap(err, "Invalid address url")
	}
	if url.Host == "" {
		return errors.Errorf("Invalid address url: host cannot be empty")
	}
	if !strings.HasPrefix(url.Scheme, "http") {
		return errors.Errorf("Invalid address url: scheme must be http[s]")
	}
	return nil
}

func addNodeForParams(p provision.NodeProvisioner, params provision.AddNodeOptions) (string, map[string]string, error) {
	response := make(map[string]string)
	var address string
	if params.Register {
		address = params.Metadata["address"]
		delete(params.Metadata, "address")
	} else {
		desc, _ := iaas.Describe(params.Metadata["iaas"])
		response["description"] = desc
		m, err := iaas.CreateMachine(params.Metadata)
		if err != nil {
			return address, response, err
		}
		address = m.FormatNodeAddress()
		params.CaCert = m.CaCert
		params.ClientCert = m.ClientCert
		params.ClientKey = m.ClientKey
	}
	prov, _, err := provision.FindNode(address)
	if err != provision.ErrNodeNotFound {
		if err == nil {
			return "", nil, errors.Errorf("node with address %q already exists in provisioner %q", address, prov.GetName())
		}
		return "", nil, err
	}
	err = validateNodeAddress(address)
	if err != nil {
		return address, response, err
	}
	params.Address = address
	err = p.AddNode(params)
	return address, response, err
}

// title: add node
// path: /node
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   201: Ok
//   401: Unauthorized
//   404: Not found
func addNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var params provision.AddNodeOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if templateName, ok := params.Metadata["template"]; ok {
		params.Metadata, err = iaas.ExpandTemplate(templateName, params.Metadata)
		if err != nil {
			return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
	}
	poolName := params.Metadata["pool"]
	if poolName == "" {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: "pool is required"}
	}
	if !permission.Check(t, permission.PermNodeCreate, permission.Context(permission.CtxPool, poolName)) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypeNode},
		Kind:        permission.PermNodeCreate,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, poolName)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	p, err := pool.GetPoolByName(poolName)
	if err != nil {
		return err
	}
	prov, err := p.GetProvisioner()
	if err != nil {
		return err
	}
	nodeProv, ok := prov.(provision.NodeProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "node operations"}
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	w.WriteHeader(http.StatusCreated)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	addr, response, err := addNodeForParams(nodeProv, params)
	evt.Target.Value = addr
	if err != nil {
		if desc := response["description"]; desc != "" {
			return errors.Wrapf(err, "Instructions:\n%s", desc)
		}
		return err
	}
	return nil
}

// title: remove node
// path: /{provisioner}/node/{address}
// method: DELETE
// responses:
//   200: Ok
//   401: Unauthorized
//   404: Not found
func removeNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	address := r.URL.Query().Get(":address")
	if address == "" {
		return errors.Errorf("Node address is required.")
	}
	prov, node, err := provision.FindNode(address)
	if err != nil {
		if err == provision.ErrNodeNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	nodeProv := prov.(provision.NodeProvisioner)
	pool := node.Pool()
	allowedNodeRemove := permission.Check(t, permission.PermNodeDelete,
		permission.Context(permission.CtxPool, pool),
	)
	if !allowedNodeRemove {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeNode, Value: node.Address()},
		Kind:       permission.PermNodeDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, pool)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	noRebalance, _ := strconv.ParseBool(r.URL.Query().Get("no-rebalance"))
	err = nodeProv.RemoveNode(provision.RemoveNodeOptions{
		Address:   address,
		Rebalance: !noRebalance,
		Writer:    w,
	})
	if err != nil {
		return err
	}
	removeIaaS, _ := strconv.ParseBool(r.URL.Query().Get("remove-iaas"))
	if removeIaaS {
		var m iaas.Machine
		m, err = iaas.FindMachineByIdOrAddress(node.Metadata()["iaas-id"], net.URLToHost(address))
		if err != nil && err != mgo.ErrNotFound {
			return nil
		}
		return m.Destroy()
	}
	return nil
}

type listNodeResponse struct {
	Nodes    []provision.NodeSpec `json:"nodes"`
	Machines []iaas.Machine       `json:"machines"`
}

// title: list nodes
// path: /{provisioner}/node
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
func listNodesHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := permission.ListContextValues(t, permission.PermNodeRead, false)
	if err != nil {
		return err
	}
	provs, err := provision.Registry()
	if err != nil {
		return err
	}
	provNameMap := map[string]string{}
	var allNodes []provision.NodeSpec
	for _, prov := range provs {
		nodeProv, ok := prov.(provision.NodeProvisioner)
		if !ok {
			continue
		}
		var nodes []provision.Node
		nodes, err = nodeProv.ListNodes(nil)
		if err != nil {
			allNodes = append(allNodes, provision.NodeSpec{
				Address: fmt.Sprintf("%s nodes", prov.GetName()),
				Status:  fmt.Sprintf("ERROR: %v", err),
			})
			continue
		}
		for _, n := range nodes {
			provNameMap[n.Address()] = prov.GetName()
			allNodes = append(allNodes, provision.NodeToSpec(n))
		}
	}
	if pools != nil {
		filteredNodes := make([]provision.NodeSpec, 0, len(allNodes))
		for _, node := range allNodes {
			for _, pool := range pools {
				if node.Pool == pool {
					filteredNodes = append(filteredNodes, node)
					break
				}
			}
		}
		allNodes = filteredNodes
	}
	iaases, err := permission.ListContextValues(t, permission.PermMachineRead, false)
	if err != nil {
		return err
	}
	machines, err := iaas.ListMachines()
	if err != nil {
		return err
	}
	if iaases != nil {
		filteredMachines := make([]iaas.Machine, 0, len(machines))
		for _, machine := range machines {
			for _, iaas := range iaases {
				if machine.Iaas == iaas {
					filteredMachines = append(filteredMachines, machine)
					break
				}
			}
		}
		machines = filteredMachines
	}
	if len(allNodes) == 0 && len(machines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	result := listNodeResponse{
		Nodes:    allNodes,
		Machines: machines,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

// title: update nodes
// path: /{provisioner}/node
// method: PUT
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
//   404: Not found
func updateNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var params provision.UpdateNodeOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if params.Disable && params.Enable {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "A node can't be enabled and disabled simultaneously.",
		}
	}
	if params.Address == "" {
		return &tsuruErrors.HTTP{Code: http.StatusBadRequest, Message: "address is required"}
	}
	prov, node, err := provision.FindNode(params.Address)
	if err != nil {
		if err == provision.ErrNodeNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	nodeProv := prov.(provision.NodeProvisioner)
	oldPool := node.Pool()
	allowedOldPool := permission.Check(t, permission.PermNodeUpdate,
		permission.Context(permission.CtxPool, oldPool),
	)
	if !allowedOldPool {
		return permission.ErrUnauthorized
	}
	newPool, ok := params.Metadata["pool"]
	if ok {
		allowedNewPool := permission.Check(t, permission.PermNodeUpdate,
			permission.Context(permission.CtxPool, newPool),
		)
		if !allowedNewPool {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeNode, Value: node.Address()},
		Kind:       permission.PermNodeUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed: event.Allowed(permission.PermPoolReadEvents,
			permission.Context(permission.CtxPool, oldPool),
			permission.Context(permission.CtxPool, newPool),
		),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return nodeProv.UpdateNode(params)
}

// title: list units by node
// path: /{provisioner}/node/{address}/containers
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
//   404: Not found
func listUnitsByNode(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	address := r.URL.Query().Get(":address")
	_, node, err := provision.FindNode(address)
	if err != nil {
		if err == provision.ErrNodeNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	hasAccess := permission.Check(t, permission.PermNodeRead,
		permission.Context(permission.CtxPool, node.Pool()))
	if !hasAccess {
		return permission.ErrUnauthorized
	}
	units, err := node.Units()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(units)
}

// title: list units by app
// path: /docker/node/apps/{appname}/containers
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
//   404: Not found
func listUnitsByApp(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get(":appname")
	a, err := app.GetByName(appName)
	if err != nil {
		if err == app.ErrAppNotFound {
			return &tsuruErrors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		return err
	}
	canRead := permission.Check(t, permission.PermAppRead,
		contextsForApp(a)...,
	)
	if !canRead {
		return permission.ErrUnauthorized
	}
	units, err := a.Units()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(units)
}

// title: node healing info
// path: /healing/node
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func nodeHealingRead(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := permission.ListContextValues(t, permission.PermHealingRead, true)
	if err != nil {
		return err
	}
	configMap, err := healer.GetConfig()
	if err != nil {
		return err
	}
	if len(pools) > 0 {
		allowedPoolSet := map[string]struct{}{}
		for _, p := range pools {
			allowedPoolSet[p] = struct{}{}
		}
		for k := range configMap {
			if k == "" {
				continue
			}
			if _, ok := allowedPoolSet[k]; !ok {
				delete(configMap, k)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(configMap)
}

// title: node healing update
// path: /healing/node
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   401: Unauthorized
func nodeHealingUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return err
	}
	poolName := r.FormValue("pool")
	var ctxs []permission.PermissionContext
	if poolName != "" {
		ctxs = append(ctxs, permission.Context(permission.CtxPool, poolName))
	}
	if !permission.Check(t, permission.PermHealingUpdate, ctxs...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:        permission.PermHealingUpdate,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	var config healer.NodeHealerConfig
	delete(r.Form, "pool")
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&config, r.Form)
	if err != nil {
		return err
	}
	return healer.UpdateConfig(poolName, config)
}

// title: remove node healing
// path: /healing/node
// method: DELETE
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func nodeHealingDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	poolName := r.URL.Query().Get("pool")
	var ctxs []permission.PermissionContext
	if poolName != "" {
		ctxs = append(ctxs, permission.Context(permission.CtxPool, poolName))
	}
	if !permission.Check(t, permission.PermHealingDelete, ctxs...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:        permission.PermHealingDelete,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if len(r.URL.Query()["name"]) == 0 {
		return healer.RemoveConfig(poolName, "")
	}
	for _, v := range r.URL.Query()["name"] {
		err := healer.RemoveConfig(poolName, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// title: rebalance units in nodes
// path: /node/rebalance
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/x-json-stream
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func rebalanceNodesHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	r.ParseForm()
	var params provision.RebalanceNodesOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &tsuruErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	params.Force = true
	var permContexts []permission.PermissionContext
	poolName, ok := params.MetadataFilter["pool"]
	if ok {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, poolName))
	}
	if !permission.Check(t, permission.PermNodeUpdateRebalance, permContexts...) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool, Value: poolName},
		Kind:        permission.PermNodeUpdateRebalance,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, permContexts...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	params.Writer = writer
	var provs []provision.Provisioner
	if poolName != "" {
		var p *pool.Pool
		var prov provision.Provisioner
		p, err = pool.GetPoolByName(poolName)
		if err != nil {
			return err
		}
		prov, err = p.GetProvisioner()
		if err != nil {
			return err
		}
		if _, ok := prov.(provision.NodeRebalanceProvisioner); !ok {
			return provision.ProvisionerNotSupported{Prov: prov, Action: "node rebalance operations"}
		}
		provs = append(provs, prov)
	} else {
		provs, err = provision.Registry()
		if err != nil {
			return err
		}
	}
	for _, prov := range provs {
		rebalanceProv, ok := prov.(provision.NodeRebalanceProvisioner)
		if !ok {
			continue
		}
		_, err = rebalanceProv.RebalanceNodes(params)
		if err != nil {
			return errors.Wrap(err, "Error trying to rebalance units in nodes")
		}
	}
	fmt.Fprintf(writer, "Units successfully rebalanced!\n")
	return nil
}
