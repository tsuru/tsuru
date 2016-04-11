// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	stderror "errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cezarsa/form"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	_ "github.com/tsuru/tsuru/iaas/cloudstack"
	_ "github.com/tsuru/tsuru/iaas/digitalocean"
	_ "github.com/tsuru/tsuru/iaas/ec2"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/docker/container"
	"github.com/tsuru/tsuru/provision/docker/healer"
	"github.com/tsuru/tsuru/provision/docker/nodecontainer"
	"github.com/tsuru/tsuru/queue"
	"gopkg.in/mgo.v2"
)

func init() {
	api.RegisterHandler("/docker/node", "GET", api.AuthorizationRequiredHandler(listNodesHandler))
	api.RegisterHandler("/docker/node/apps/{appname}/containers", "GET", api.AuthorizationRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node/{address:.*}/containers", "GET", api.AuthorizationRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node", "POST", api.AuthorizationRequiredHandler(addNodeHandler))
	api.RegisterHandler("/docker/node", "PUT", api.AuthorizationRequiredHandler(updateNodeHandler))
	api.RegisterHandler("/docker/node", "DELETE", api.AuthorizationRequiredHandler(removeNodeHandler))
	api.RegisterHandler("/docker/container/{id}/move", "POST", api.AuthorizationRequiredHandler(moveContainerHandler))
	api.RegisterHandler("/docker/containers/move", "POST", api.AuthorizationRequiredHandler(moveContainersHandler))
	api.RegisterHandler("/docker/containers/rebalance", "POST", api.AuthorizationRequiredHandler(rebalanceContainersHandler))
	api.RegisterHandler("/docker/healing", "GET", api.AuthorizationRequiredHandler(healingHistoryHandler))
	api.RegisterHandler("/docker/healing/node", "GET", api.AuthorizationRequiredHandler(nodeHealingRead))
	api.RegisterHandler("/docker/healing/node", "POST", api.AuthorizationRequiredHandler(nodeHealingUpdate))
	api.RegisterHandler("/docker/healing/node", "DELETE", api.AuthorizationRequiredHandler(nodeHealingDelete))
	api.RegisterHandler("/docker/autoscale", "GET", api.AuthorizationRequiredHandler(autoScaleHistoryHandler))
	api.RegisterHandler("/docker/autoscale/config", "GET", api.AuthorizationRequiredHandler(autoScaleGetConfig))
	api.RegisterHandler("/docker/autoscale/run", "POST", api.AuthorizationRequiredHandler(autoScaleRunHandler))
	api.RegisterHandler("/docker/autoscale/rules", "GET", api.AuthorizationRequiredHandler(autoScaleListRules))
	api.RegisterHandler("/docker/autoscale/rules", "POST", api.AuthorizationRequiredHandler(autoScaleSetRule))
	api.RegisterHandler("/docker/autoscale/rules/", "DELETE", api.AuthorizationRequiredHandler(autoScaleDeleteRule))
	api.RegisterHandler("/docker/autoscale/rules/{id}", "DELETE", api.AuthorizationRequiredHandler(autoScaleDeleteRule))
	api.RegisterHandler("/docker/bs/upgrade", "POST", api.AuthorizationRequiredHandler(bsUpgradeHandler))
	api.RegisterHandler("/docker/bs/env", "POST", api.AuthorizationRequiredHandler(bsEnvSetHandler))
	api.RegisterHandler("/docker/bs", "GET", api.AuthorizationRequiredHandler(bsConfigGetHandler))
	api.RegisterHandler("/docker/nodecontainers", "GET", api.AuthorizationRequiredHandler(nodeContainerList))
	api.RegisterHandler("/docker/nodecontainers", "POST", api.AuthorizationRequiredHandler(nodeContainerCreate))
	api.RegisterHandler("/docker/nodecontainers/{name}", "GET", api.AuthorizationRequiredHandler(nodeContainerInfo))
	api.RegisterHandler("/docker/nodecontainers/{name}", "DELETE", api.AuthorizationRequiredHandler(nodeContainerDelete))
	api.RegisterHandler("/docker/nodecontainers/{name}", "POST", api.AuthorizationRequiredHandler(nodeContainerUpdate))
	api.RegisterHandler("/docker/nodecontainers/{name}/upgrade", "POST", api.AuthorizationRequiredHandler(nodeContainerUpgrade))
	api.RegisterHandler("/docker/logs", "GET", api.AuthorizationRequiredHandler(logsConfigGetHandler))
	api.RegisterHandler("/docker/logs", "POST", api.AuthorizationRequiredHandler(logsConfigSetHandler))
}

func autoScaleGetConfig(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedGetConfig := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedGetConfig {
		return permission.ErrUnauthorized
	}
	config := mainDockerProvisioner.initAutoScaleConfig()
	return json.NewEncoder(w).Encode(config)
}

func autoScaleListRules(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedListRule := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedListRule {
		return permission.ErrUnauthorized
	}
	rules, err := listAutoScaleRules()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&rules)
}

func autoScaleSetRule(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedSetRule := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedSetRule {
		return permission.ErrUnauthorized
	}
	var rule autoScaleRule
	err := json.NewDecoder(r.Body).Decode(&rule)
	if err != nil {
		return err
	}
	return rule.update()
}

func autoScaleDeleteRule(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedDeleteRule := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedDeleteRule {
		return permission.ErrUnauthorized
	}
	ruleID := r.URL.Query().Get(":id")
	err := deleteAutoScaleRule(ruleID)
	if err == mgo.ErrNotFound {
		return &errors.HTTP{Code: http.StatusNotFound, Message: "rule not found"}
	}
	return nil
}

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

func (p *dockerProvisioner) addNodeForParams(params map[string]string, isRegister bool) (map[string]string, error) {
	response := make(map[string]string)
	var machineID string
	var address string
	if isRegister {
		address, _ = params["address"]
		delete(params, "address")
	} else {
		desc, _ := iaas.Describe(params["iaas"])
		response["description"] = desc
		m, err := iaas.CreateMachine(params)
		if err != nil {
			return response, err
		}
		address = m.FormatNodeAddress()
		machineID = m.Id
	}
	err := validateNodeAddress(address)
	if err != nil {
		return response, err
	}
	node := cluster.Node{Address: address, Metadata: params, CreationStatus: cluster.NodeCreationStatusPending}
	err = p.Cluster().Register(node)
	if err != nil {
		return response, err
	}
	q, err := queue.Queue()
	if err != nil {
		return response, err
	}
	jobParams := monsterqueue.JobParams{"endpoint": address, "machine": machineID, "metadata": params}
	_, err = q.Enqueue(nodecontainer.QueueTaskName, jobParams)
	return response, err
}

// addNodeHandler can provide an machine and/or register a node address.
// If register flag is true, it will just register a node.
// It checks if node address is valid and accessible.
func addNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	if templateName, ok := params["template"]; ok {
		params, err = iaas.ExpandTemplate(templateName)
		if err != nil {
			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
		}
	}
	pool := params["pool"]
	if pool == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "pool is required"}
	}
	if !permission.Check(t, permission.PermNodeCreate, permission.Context(permission.CtxPool, pool)) {
		return permission.ErrUnauthorized
	}
	isRegister, _ := strconv.ParseBool(r.URL.Query().Get("register"))
	if !isRegister {
		canCreateMachine := permission.Check(t, permission.PermMachineCreate,
			permission.Context(permission.CtxIaaS, params["iaas"]))
		if !canCreateMachine {
			return permission.ErrUnauthorized
		}
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	response, err := mainDockerProvisioner.addNodeForParams(params, isRegister)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{
			Error: fmt.Sprintf("%s\n\n%s", err, response["description"]),
		})
	}
	return nil
}

// removeNodeHandler calls scheduler.Unregister to unregistering a node into it.
func removeNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	address, _ := params["address"]
	if address == "" {
		return fmt.Errorf("Node address is required.")
	}
	node, err := mainDockerProvisioner.Cluster().GetNode(address)
	if err != nil {
		return err
	}
	allowedNodeRemove := permission.Check(t, permission.PermNodeDelete,
		permission.Context(permission.CtxPool, node.Metadata["pool"]),
	)
	if !allowedNodeRemove {
		return permission.ErrUnauthorized
	}
	removeIaaS, _ := strconv.ParseBool(params["remove_iaas"])
	if removeIaaS {
		allowedIaasRemove := permission.Check(t, permission.PermMachineDelete,
			permission.Context(permission.CtxIaaS, node.Metadata["iaas"]),
		)
		if !allowedIaasRemove {
			return permission.ErrUnauthorized
		}
	}
	node.CreationStatus = cluster.NodeCreationStatusDisabled
	_, err = mainDockerProvisioner.Cluster().UpdateNode(node)
	if err != nil {
		return err
	}
	noRebalance, err := strconv.ParseBool(r.URL.Query().Get("no-rebalance"))
	if !noRebalance {
		err = mainDockerProvisioner.rebalanceContainersByHost(net.URLToHost(address), w)
		if err != nil {
			return err
		}
	}
	err = mainDockerProvisioner.Cluster().Unregister(address)
	if err != nil {
		return err
	}
	if removeIaaS {
		var m iaas.Machine
		m, err = iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], net.URLToHost(address))
		if err != nil && err != mgo.ErrNotFound {
			return err
		}
		return m.Destroy()
	}
	return nil
}

func listNodesHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := listContextValues(t, permission.PermNodeRead, false)
	if err != nil {
		return err
	}
	nodes, err := mainDockerProvisioner.Cluster().UnfilteredNodes()
	if err != nil {
		return err
	}
	if pools != nil {
		filteredNodes := make([]cluster.Node, 0, len(nodes))
		for _, node := range nodes {
			for _, pool := range pools {
				if node.Metadata["pool"] == pool {
					filteredNodes = append(filteredNodes, node)
					break
				}
			}
		}
		nodes = filteredNodes
	}
	iaases, err := listContextValues(t, permission.PermMachineRead, false)
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
	result := map[string]interface{}{
		"nodes":    nodes,
		"machines": machines,
	}
	return json.NewEncoder(w).Encode(result)
}

func updateNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	address, _ := params["address"]
	if address == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "address is required"}
	}
	oldNode, err := mainDockerProvisioner.Cluster().GetNode(address)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: err.Error(),
		}
	}
	oldPool, _ := oldNode.Metadata["pool"]
	allowedOldPool := permission.Check(t, permission.PermNodeUpdate,
		permission.Context(permission.CtxPool, oldPool),
	)
	if !allowedOldPool {
		return permission.ErrUnauthorized
	}
	newPool, ok := params["pool"]
	if ok {
		allowedNewPool := permission.Check(t, permission.PermNodeUpdate,
			permission.Context(permission.CtxPool, newPool),
		)
		if !allowedNewPool {
			return permission.ErrUnauthorized
		}
	}
	delete(params, "address")
	node := cluster.Node{Address: address, Metadata: params}
	disabled, _ := strconv.ParseBool(r.URL.Query().Get("disabled"))
	enabled, _ := strconv.ParseBool(r.URL.Query().Get("enabled"))
	if disabled && enabled {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "You can't make a node enable and disable at the same time.",
		}
	}
	if disabled {
		node.CreationStatus = cluster.NodeCreationStatusDisabled
	}
	if enabled {
		node.CreationStatus = cluster.NodeCreationStatusCreated
	}
	_, err = mainDockerProvisioner.Cluster().UpdateNode(node)
	return err
}

func moveContainerHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	contId := r.URL.Query().Get(":id")
	to := params["to"]
	if to == "" {
		return fmt.Errorf("Invalid params: id: %s - to: %s", contId, to)
	}
	cont, err := mainDockerProvisioner.GetContainer(contId)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	permContexts, err := moveContainersPermissionContexts(cont.HostAddr, to)
	if err != nil {
		return err
	}
	if !permission.Check(t, permission.PermNode, permContexts...) {
		return permission.ErrUnauthorized
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	_, err = mainDockerProvisioner.moveContainer(contId, to, writer)
	if err != nil {
		fmt.Fprintf(writer, "Error trying to move container: %s\n", err.Error())
	} else {
		fmt.Fprintf(writer, "Containers moved successfully!\n")
	}
	return nil
}

func moveContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	from := params["from"]
	to := params["to"]
	if from == "" || to == "" {
		return fmt.Errorf("Invalid params: from: %s - to: %s", from, to)
	}
	permContexts, err := moveContainersPermissionContexts(from, to)
	if err != nil {
		return err
	}
	if !permission.Check(t, permission.PermNode, permContexts...) {
		return permission.ErrUnauthorized
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = mainDockerProvisioner.MoveContainers(from, to, writer)
	if err != nil {
		fmt.Fprintf(writer, "Error trying to move containers: %s\n", err.Error())
	} else {
		fmt.Fprintf(writer, "Containers moved successfully!\n")
	}
	return nil
}

func moveContainersPermissionContexts(from, to string) ([]permission.PermissionContext, error) {
	originHost, err := mainDockerProvisioner.getNodeByHost(from)
	if err != nil {
		return nil, &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	destinationHost, err := mainDockerProvisioner.getNodeByHost(to)
	if err != nil {
		return nil, &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	var permContexts []permission.PermissionContext
	originPool, ok := originHost.Metadata["pool"]
	if ok {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, originPool))
	}
	if pool, ok := destinationHost.Metadata["pool"]; ok && pool != originPool {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, pool))
	}
	return permContexts, nil
}

func rebalanceContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	var dry bool
	var params struct {
		Dry            string
		MetadataFilter map[string]string
		AppFilter      []string
	}
	err := json.NewDecoder(r.Body).Decode(&params)
	if err == nil {
		dry, _ = strconv.ParseBool(params.Dry)
	}
	var permContexts []permission.PermissionContext
	if pool, ok := params.MetadataFilter["pool"]; ok {
		permContexts = append(permContexts, permission.Context(permission.CtxPool, pool))
	}
	if !permission.Check(t, permission.PermNode, permContexts...) {
		return permission.ErrUnauthorized
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	_, err = mainDockerProvisioner.rebalanceContainersByFilter(writer, params.AppFilter, params.MetadataFilter, dry)
	if err != nil {
		fmt.Fprintf(writer, "Error trying to rebalance containers: %s\n", err)
	} else {
		fmt.Fprintf(writer, "Containers successfully rebalanced!\n")
	}
	return nil
}

func listContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	address := r.URL.Query().Get(":address")
	if address != "" {
		node, err := mainDockerProvisioner.Cluster().GetNode(address)
		if err != nil {
			return err
		}
		hasAccess := permission.Check(t, permission.PermNodeRead,
			permission.Context(permission.CtxPool, node.Metadata["pool"]))
		if !hasAccess {
			return permission.ErrUnauthorized
		}
		containerList, err := mainDockerProvisioner.listContainersByHost(net.URLToHost(address))
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(containerList)
	}
	appName := r.URL.Query().Get(":appname")
	a, err := app.GetByName(appName)
	if err != nil {
		return err
	}
	hasAccess := permission.Check(t, permission.PermNodeRead,
		permission.Context(permission.CtxPool, a.Pool))
	if !hasAccess {
		return permission.ErrUnauthorized
	}
	containerList, err := mainDockerProvisioner.listContainersByApp(appName)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(containerList)
}

func unmarshal(body io.ReadCloser) (map[string]string, error) {
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	err = json.Unmarshal(b, &params)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func healingHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermHealingRead) {
		return permission.ErrUnauthorized
	}
	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "node" && filter != "container" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "invalid filter, possible values are 'node' or 'container'",
		}
	}
	history, err := healer.ListHealingHistory(filter)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(history)
}

func autoScaleHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermNodeAutoscale) {
		return permission.ErrUnauthorized
	}
	skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	history, err := listAutoScaleEvents(skip, limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&history)
}

func autoScaleRunHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermNodeAutoscale) {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(keepAliveWriter),
	}
	autoScaleConfig := mainDockerProvisioner.initAutoScaleConfig()
	autoScaleConfig.writer = writer
	err := autoScaleConfig.runOnce()
	if err != nil {
		writer.Encoder.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
	}
	return nil
}

func bsEnvSetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return stderror.New("this route is deprecated, please use POST /docker/nodecontainer/{name} (node-container-update command)")
}

func bsConfigGetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return stderror.New("this route is deprecated, please use GET /docker/nodecontainer/{name} (node-container-info command)")
}

func bsUpgradeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	return stderror.New("this route is deprecated, please use POST /docker/nodecontainer/{name}/upgrade (node-container-upgrade command)")
}

func listContextValues(t permission.Token, scheme *permission.PermissionScheme, failIfEmpty bool) ([]string, error) {
	contexts := permission.ContextsForPermission(t, scheme)
	if len(contexts) == 0 && failIfEmpty {
		return nil, permission.ErrUnauthorized
	}
	values := make([]string, 0, len(contexts))
	for _, ctx := range contexts {
		if ctx.CtxType == permission.CtxGlobal {
			return nil, nil
		}
		values = append(values, ctx.Value)
	}
	return values, nil
}

func logsConfigGetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := listContextValues(t, permission.PermPoolUpdateLogs, true)
	if err != nil {
		return err
	}
	configEntries, err := container.LogLoadAll()
	if err != nil {
		return err
	}
	if len(pools) == 0 {
		return json.NewEncoder(w).Encode(configEntries)
	}
	newMap := map[string]container.DockerLogConfig{}
	for _, p := range pools {
		if entry, ok := configEntries[p]; ok {
			newMap[p] = entry
		}
	}
	return json.NewEncoder(w).Encode(newMap)
}

func logsConfigSetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse form values: %s", err),
		}
	}
	pool := r.Form.Get("pool")
	restart, _ := strconv.ParseBool(r.Form.Get("restart"))
	delete(r.Form, "pool")
	delete(r.Form, "restart")
	var conf container.DockerLogConfig
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	err = dec.DecodeValues(&conf, r.Form)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse fields in docker log config: %s", err),
		}
	}
	if pool == "" && !permission.Check(t, permission.PermPoolUpdateLogs) {
		return permission.ErrUnauthorized
	}
	hasPermission := permission.Check(t, permission.PermPoolUpdateLogs,
		permission.Context(permission.CtxPool, pool))
	if !hasPermission {
		return permission.ErrUnauthorized
	}
	err = conf.Save(pool)
	if err != nil {
		return err
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	fmt.Fprintln(writer, "Log config successfully updated.")
	if restart {
		filter := &app.Filter{}
		if pool != "" {
			filter.Pools = []string{pool}
		}
		tryRestartAppsByFilter(filter, writer)
	}
	return nil
}

func tryRestartAppsByFilter(filter *app.Filter, writer *tsuruIo.SimpleJsonMessageEncoderWriter) {
	apps, err := app.List(filter)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
		return
	}
	if len(apps) == 0 {
		return
	}
	appNames := make([]string, len(apps))
	for i, a := range apps {
		appNames[i] = a.Name
	}
	sort.Strings(appNames)
	fmt.Fprintf(writer, "Restarting %d applications: [%s]\n", len(apps), strings.Join(appNames, ", "))
	wg := sync.WaitGroup{}
	for i := range apps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := apps[i]
			err := a.Restart("", writer)
			if err != nil {
				fmt.Fprintf(writer, "Error: unable to restart %s: %s\n", a.Name, err.Error())
			} else {
				fmt.Fprintf(writer, "App %s successfully restarted\n", a.Name)
			}
		}(i)
	}
	wg.Wait()
}

func nodeHealingRead(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := listContextValues(t, permission.PermHealingRead, true)
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

func nodeHealingUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermHealingUpdate) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermHealingUpdate,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
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

func nodeHealingDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermHealingUpdate) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermHealingUpdate,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
	if len(r.Form["name"]) == 0 {
		return healer.RemoveConfig(poolName, "")
	}
	for _, v := range r.Form["name"] {
		err = healer.RemoveConfig(poolName, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func nodeContainerList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := listContextValues(t, permission.PermNodecontainerRead, true)
	if err != nil {
		return err
	}
	lst, err := nodecontainer.AllNodeContainers()
	if err != nil {
		return err
	}
	if pools != nil {
		poolMap := map[string]struct{}{}
		for _, p := range pools {
			poolMap[p] = struct{}{}
		}
		for i, entry := range lst {
			for poolName := range entry.ConfigPools {
				if poolName == "" {
					continue
				}
				if _, ok := poolMap[poolName]; !ok {
					delete(entry.ConfigPools, poolName)
				}
			}
			lst[i] = entry
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(lst)
}

func nodeContainerCreate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermNodecontainerCreate) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermNodecontainerCreate,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	var config nodecontainer.NodeContainerConfig
	err = dec.DecodeValues(&config, r.Form)
	if err != nil {
		return err
	}
	err = nodecontainer.AddNewContainer(poolName, &config)
	if err != nil {
		if _, ok := err.(nodecontainer.ValidationErr); ok {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
		return err
	}
	return nil
}

func nodeContainerInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	pools, err := listContextValues(t, permission.PermNodecontainerRead, true)
	if err != nil {
		return err
	}
	name := r.URL.Query().Get(":name")
	configMap, err := nodecontainer.LoadNodeContainersForPools(name)
	if err != nil {
		return err
	}
	if pools != nil {
		poolMap := map[string]struct{}{}
		for _, p := range pools {
			poolMap[p] = struct{}{}
		}
		for poolName := range configMap {
			if poolName == "" {
				continue
			}
			if _, ok := poolMap[poolName]; !ok {
				delete(configMap, poolName)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(configMap)
}

func nodeContainerUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermNodecontainerUpdate) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermNodecontainerUpdate,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreUnknownKeys(true)
	dec.IgnoreCase(true)
	var config nodecontainer.NodeContainerConfig
	err = dec.DecodeValues(&config, r.Form)
	if err != nil {
		return err
	}
	config.Name = r.URL.Query().Get(":name")
	err = nodecontainer.UpdateContainer(poolName, &config)
	if err != nil {
		if err == nodecontainer.ErrNodeContainerNotFound {
			return &errors.HTTP{
				Code:    http.StatusNotFound,
				Message: err.Error(),
			}
		}
		if _, ok := err.(nodecontainer.ValidationErr); ok {
			return &errors.HTTP{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			}
		}
		return err
	}
	return nil
}

func nodeContainerDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermNodecontainerDelete) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermNodecontainerDelete,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
	err := nodecontainer.RemoveContainer(poolName, name)
	if err == nodecontainer.ErrNodeContainerNotFound {
		return &errors.HTTP{
			Code:    http.StatusNotFound,
			Message: fmt.Sprintf("node container %q not found for pool %q", name, poolName),
		}
	}
	return err
}

func nodeContainerUpgrade(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	poolName := r.FormValue("pool")
	if poolName == "" {
		if !permission.Check(t, permission.PermNodecontainerUpdateUpgrade) {
			return permission.ErrUnauthorized
		}
	} else {
		if !permission.Check(t, permission.PermNodecontainerUpdateUpgrade,
			permission.Context(permission.CtxPool, poolName)) {
			return permission.ErrUnauthorized
		}
	}
	err := nodecontainer.ResetImage(poolName, name)
	if err != nil {
		return err
	}
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	err = nodecontainer.RecreateNamedContainers(mainDockerProvisioner, writer, name)
	if err != nil {
		writer.Encode(tsuruIo.SimpleJsonMessage{Error: err.Error()})
	}
	return nil
}
