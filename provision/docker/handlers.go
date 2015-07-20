// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	_ "github.com/tsuru/tsuru/iaas/cloudstack"
	_ "github.com/tsuru/tsuru/iaas/ec2"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/queue"
	"gopkg.in/mgo.v2"
)

func init() {
	api.RegisterHandler("/docker/node", "GET", api.AdminRequiredHandler(listNodeHandler))
	api.RegisterHandler("/docker/node/apps/{appname}/containers", "GET", api.AdminRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node/{address}/containers", "GET", api.AdminRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node", "POST", api.AdminRequiredHandler(addNodeHandler))
	api.RegisterHandler("/docker/node", "PUT", api.AdminRequiredHandler(updateNodeHandler))
	api.RegisterHandler("/docker/node", "DELETE", api.AdminRequiredHandler(removeNodeHandler))
	api.RegisterHandler("/docker/container/{id}/move", "POST", api.AdminRequiredHandler(moveContainerHandler))
	api.RegisterHandler("/docker/containers/move", "POST", api.AdminRequiredHandler(moveContainersHandler))
	api.RegisterHandler("/docker/containers/rebalance", "POST", api.AdminRequiredHandler(rebalanceContainersHandler))
	api.RegisterHandler("/docker/fix-containers", "POST", api.AdminRequiredHandler(fixContainersHandler))
	api.RegisterHandler("/docker/healing", "GET", api.AdminRequiredHandler(healingHistoryHandler))
	api.RegisterHandler("/docker/autoscale", "GET", api.AdminRequiredHandler(autoScaleHistoryHandler))
	api.RegisterHandler("/docker/autoscale/run", "POST", api.AdminRequiredHandler(autoScaleRunHandler))
	api.RegisterHandler("/docker/bs/upgrade", "POST", api.AdminRequiredHandler(bsUpgradeHandler))
	api.RegisterHandler("/docker/bs/env", "POST", api.AdminRequiredHandler(bsEnvSetHandler))
	api.RegisterHandler("/docker/bs", "GET", api.AdminRequiredHandler(bsConfigGetHandler))
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
	err = p.getCluster().Register(node)
	if err != nil {
		return response, err
	}
	q, err := queue.Queue()
	if err != nil {
		return response, err
	}
	jobParams := monsterqueue.JobParams{"endpoint": address, "machine": machineID, "metadata": params}
	_, err = q.Enqueue(runBsTaskName, jobParams)
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
	isRegister, _ := strconv.ParseBool(r.URL.Query().Get("register"))
	response, err := mainDockerProvisioner.addNodeForParams(params, isRegister)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
	}
	return json.NewEncoder(w).Encode(response)
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
	nodes, err := mainDockerProvisioner.getCluster().UnfilteredNodes()
	if err != nil {
		return err
	}
	var node *cluster.Node
	for i := range nodes {
		if nodes[i].Address == address {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		return fmt.Errorf("node with address %q not found in cluster", address)
	}
	err = mainDockerProvisioner.getCluster().Unregister(address)
	if err != nil {
		return err
	}
	removeIaaS, _ := strconv.ParseBool(params["remove_iaas"])
	if removeIaaS {
		m, err := iaas.FindMachineByIdOrAddress(node.Metadata["iaas-id"], urlToHost(address))
		if err != nil && err != mgo.ErrNotFound {
			return err
		}
		return m.Destroy()
	}
	return nil
}

//listNodeHandler call scheduler.Nodes to list all nodes into it.
func listNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	nodeList, err := mainDockerProvisioner.getCluster().UnfilteredNodes()
	if err != nil {
		return err
	}
	machines, err := iaas.ListMachines()
	if err != nil {
		return err
	}
	result := map[string]interface{}{
		"nodes":    nodeList,
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
	delete(params, "address")
	node := cluster.Node{Address: address, Metadata: params}
	_, err = mainDockerProvisioner.getCluster().UpdateNode(node)
	return err
}

func fixContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := mainDockerProvisioner.fixContainers()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
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
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(w),
	}
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
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(w),
	}
	err = mainDockerProvisioner.moveContainers(from, to, writer)
	if err != nil {
		fmt.Fprintf(writer, "Error trying to move containers: %s\n", err.Error())
	} else {
		fmt.Fprintf(writer, "Containers moved successfully!\n")
	}
	return nil
}

func rebalanceContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	dry := false
	params := struct {
		Dry            string
		MetadataFilter map[string]string
		AppFilter      []string
	}{}
	err := json.NewDecoder(r.Body).Decode(&params)
	if err == nil {
		dry, _ = strconv.ParseBool(params.Dry)
	}
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(w),
	}
	_, err = mainDockerProvisioner.rebalanceContainersByFilter(writer, params.AppFilter, params.MetadataFilter, dry)
	if err != nil {
		fmt.Fprintf(writer, "Error trying to rebalance containers: %s\n", err.Error())
	} else {
		fmt.Fprintf(writer, "Containers rebalanced successfully!\n")
	}
	return nil
}

//listContainersHandler call scheduler.Containers to list all containers into it.
func listContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	address := r.URL.Query().Get(":address")
	if address != "" {
		containerList, err := mainDockerProvisioner.listContainersByHost(address)
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(containerList)
	}
	app := r.URL.Query().Get(":appname")
	containerList, err := mainDockerProvisioner.listContainersByApp(app)
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
	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "node" && filter != "container" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "invalid filter, possible values are 'node' or 'container'",
		}
	}
	history, err := listHealingHistory(filter)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(history)
}

func autoScaleHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	history, err := listAutoScaleEvents(skip, limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(&history)
}

func autoScaleRunHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(w),
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
	var requestConfig bsConfig
	err := json.NewDecoder(r.Body).Decode(&requestConfig)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("unable to parse body as json: %s", err),
		}
	}
	currentConfig, err := loadBsConfig()
	if err != nil {
		if err != mgo.ErrNotFound {
			return err
		}
		currentConfig = &bsConfig{}
	}
	envMap := bsEnvMap{}
	poolEnvMap := bsPoolEnvMap{}
	err = currentConfig.updateEnvMaps(envMap, poolEnvMap)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	err = requestConfig.updateEnvMaps(envMap, poolEnvMap)
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		}
	}
	err = saveBsEnvs(envMap, poolEnvMap)
	if err != nil {
		return err
	}
	err = mainDockerProvisioner.recreateBsContainers()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func bsConfigGetHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	currentConfig, err := loadBsConfig()
	if err != nil {
		if err != mgo.ErrNotFound {
			return err
		}
		currentConfig = &bsConfig{}
	}
	return json.NewEncoder(w).Encode(currentConfig)
}

func bsUpgradeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := saveBsImage("")
	if err != nil {
		return err
	}
	err = mainDockerProvisioner.recreateBsContainers()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
