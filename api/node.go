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
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/iaas"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"gopkg.in/mgo.v2"
)

func validateNodeAddress(address string) error {
	if address == "" {
		return fmt.Errorf("address=url parameter is required")
	}
	url, err := url.ParseRequestURI(address)
	if err != nil {
		return fmt.Errorf("Invalid address url: %s", err.Error())
	}
	if url.Host == "" {
		return fmt.Errorf("Invalid address url: host cannot be empty")
	}
	if !strings.HasPrefix(url.Scheme, "http") {
		return fmt.Errorf("Invalid address url: scheme must be http[s]")
	}
	return nil
}

func addNodeForParams(p provision.NodeProvisioner, params provision.AddNodeOptions) (string, map[string]string, error) {
	response := make(map[string]string)
	var address string
	if params.Register {
		address, _ = params.Metadata["address"]
		delete(params.Metadata, "address")
	} else {
		desc, _ := iaas.Describe(params.Metadata["iaas"])
		response["description"] = desc
		m, err := iaas.CreateMachine(params.Metadata)
		if err != nil {
			return address, response, err
		}
		address = m.FormatNodeAddress()
	}
	err := validateNodeAddress(address)
	if err != nil {
		return address, response, err
	}
	params.Address = address
	err = p.AddNode(params)
	return address, response, err
}

// title: add node
// path: /{provisioner}/node
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
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var params provision.AddNodeOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if templateName, ok := params.Metadata["template"]; ok {
		params.Metadata, err = iaas.ExpandTemplate(templateName, params.Metadata)
		if err != nil {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
	}
	pool := params.Metadata["pool"]
	if pool == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "pool is required"}
	}
	if !permission.Check(t, permission.PermNodeCreate, permission.Context(permission.CtxPool, pool)) {
		return permission.ErrUnauthorized
	}
	if !params.Register {
		canCreateMachine := permission.Check(t, permission.PermMachineCreate,
			permission.Context(permission.CtxIaaS, params.Metadata["iaas"]))
		if !canCreateMachine {
			return permission.ErrUnauthorized
		}
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypeNode},
		Kind:        permission.PermNodeCreate,
		Owner:       t,
		CustomData:  event.FormToCustomData(r.Form),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents, permission.Context(permission.CtxPool, pool)),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	prov, err := provision.Get(r.URL.Query().Get(":provisioner"))
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
		return fmt.Errorf("%s\n\n%s", err, response["description"])
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
		return fmt.Errorf("Node address is required.")
	}
	prov, err := provision.Get(r.URL.Query().Get(":provisioner"))
	if err != nil {
		return err
	}
	nodeProv, ok := prov.(provision.NodeProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "node operations"}
	}
	node, err := nodeProv.GetNode(address)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: fmt.Sprintf("Node %s not found.", address),
		}
	}
	pool := node.Pool()
	allowedNodeRemove := permission.Check(t, permission.PermNodeDelete,
		permission.Context(permission.CtxPool, pool),
	)
	if !allowedNodeRemove {
		return permission.ErrUnauthorized
	}
	removeIaaS, _ := strconv.ParseBool(r.URL.Query().Get("remove-iaas"))
	if removeIaaS {
		allowedIaasRemove := permission.Check(t, permission.PermMachineDelete,
			permission.Context(permission.CtxIaaS, node.Metadata()["iaas"]),
		)
		if !allowedIaasRemove {
			return permission.ErrUnauthorized
		}
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
	Nodes    []json.RawMessage
	Machines []iaas.Machine
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
	prov, err := provision.Get(r.URL.Query().Get(":provisioner"))
	if err != nil {
		return err
	}
	nodeProv, ok := prov.(provision.NodeProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "node operations"}
	}
	nodes, err := nodeProv.ListNodes(nil)
	if err != nil {
		return err
	}
	if pools != nil {
		filteredNodes := make([]provision.Node, 0, len(nodes))
		for _, node := range nodes {
			for _, pool := range pools {
				if node.Pool() == pool {
					filteredNodes = append(filteredNodes, node)
					break
				}
			}
		}
		nodes = filteredNodes
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
	if len(nodes) == 0 && len(machines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	nodesJson := make([]json.RawMessage, len(nodes))
	for i, n := range nodes {
		nodesJson[i], err = provision.NodeToJSON(n)
		if err != nil {
			return err
		}
	}
	result := listNodeResponse{
		Nodes:    nodesJson,
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
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var params provision.UpdateNodeOptions
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&params, r.Form)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if params.Disable && params.Enable {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "A node can't be enabled and disabled simultaneously.",
		}
	}
	if params.Address == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "address is required"}
	}
	prov, err := provision.Get(r.URL.Query().Get(":provisioner"))
	if err != nil {
		return err
	}
	nodeProv, ok := prov.(provision.NodeProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "node operations"}
	}
	node, err := nodeProv.GetNode(params.Address)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
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
	prov, err := provision.Get(r.URL.Query().Get(":provisioner"))
	if err != nil {
		return err
	}
	nodeProv, ok := prov.(provision.NodeProvisioner)
	if !ok {
		return provision.ProvisionerNotSupported{Prov: prov, Action: "node operations"}
	}
	node, err := nodeProv.GetNode(address)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: fmt.Sprintf("Node %s not found.", address),
		}
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
			return &errors.HTTP{
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
