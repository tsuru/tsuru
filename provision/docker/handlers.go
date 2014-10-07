// Copyright 2014 tsuru authors. All rights reserved.
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

	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/iaas"
	_ "github.com/tsuru/tsuru/iaas/cloudstack"
	_ "github.com/tsuru/tsuru/iaas/ec2"
	"gopkg.in/mgo.v2"
)

func init() {
	api.RegisterHandler("/docker/node", "GET", api.AdminRequiredHandler(listNodeHandler))
	api.RegisterHandler("/docker/node/apps/{appname}/containers", "GET", api.AdminRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node/{address}/containers", "GET", api.AdminRequiredHandler(listContainersHandler))
	api.RegisterHandler("/docker/node", "POST", api.AdminRequiredHandler(addNodeHandler))
	api.RegisterHandler("/docker/node", "DELETE", api.AdminRequiredHandler(removeNodeHandler))
	api.RegisterHandler("/docker/container/{id}/move", "POST", api.AdminRequiredHandler(moveContainerHandler))
	api.RegisterHandler("/docker/containers/move", "POST", api.AdminRequiredHandler(moveContainersHandler))
	api.RegisterHandler("/docker/containers/rebalance", "POST", api.AdminRequiredHandler(rebalanceContainersHandler))
	api.RegisterHandler("/docker/pool", "GET", api.AdminRequiredHandler(listPoolHandler))
	api.RegisterHandler("/docker/pool", "POST", api.AdminRequiredHandler(addPoolHandler))
	api.RegisterHandler("/docker/pool", "DELETE", api.AdminRequiredHandler(removePoolHandler))
	api.RegisterHandler("/docker/pool/team", "POST", api.AdminRequiredHandler(addTeamToPoolHandler))
	api.RegisterHandler("/docker/pool/team", "DELETE", api.AdminRequiredHandler(removeTeamToPoolHandler))
	api.RegisterHandler("/docker/fix-containers", "POST", api.AdminRequiredHandler(fixContainersHandler))
	api.RegisterHandler("/docker/ssh/{container_id}", "GET", api.AdminRequiredHandler(sshToContainerHandler))
	api.RegisterHandler("/docker/healing", "GET", api.AdminRequiredHandler(healingHistoryHandler))
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

func addNodeForParams(params map[string]string, isRegister bool) (map[string]string, error) {
	response := make(map[string]string)
	var address string
	if isRegister {
		address, _ = params["address"]
		delete(params, "address")
	} else {
		iaasName, _ := params["iaas"]
		desc, err := iaas.Describe(iaasName)
		if err != nil {
			return response, err
		}
		response["description"] = desc
		var m *iaas.Machine
		if iaasName != "" {
			m, err = iaas.CreateMachineForIaaS(iaasName, params)
		} else {
			m, err = iaas.CreateMachine(params)
		}
		if err != nil {
			return response, err
		}
		params["iaas"] = m.Iaas
		address = m.FormatNodeAddress()
	}
	err := validateNodeAddress(address)
	if err != nil {
		return response, err
	}
	_, err = dockerCluster().Register(address, params)
	if err != nil {
		return response, err
	}
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
	response, err := addNodeForParams(params, isRegister)
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
	err = dockerCluster().Unregister(address)
	if err != nil {
		return err
	}
	removeIaaS, _ := strconv.ParseBool(params["remove_iaas"])
	if removeIaaS {
		m, err := iaas.FindMachineByAddress(urlToHost(address))
		if err != nil && err != mgo.ErrNotFound {
			return err
		}
		return m.Destroy()
	}
	return nil
}

//listNodeHandler call scheduler.Nodes to list all nodes into it.
func listNodeHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	nodeList, err := dockerCluster().UnfilteredNodes()
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

func fixContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := fixContainers()
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
	encoder := json.NewEncoder(w)
	_, err = moveContainer(contId, to, encoder)
	if err != nil {
		logProgress(encoder, "Error trying to move container: %s", err.Error())
	} else {
		logProgress(encoder, "Containers moved successfully!")
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
	encoder := json.NewEncoder(w)
	err = moveContainers(from, to, encoder)
	if err != nil {
		logProgress(encoder, "Error: %s", err.Error())
	} else {
		logProgress(encoder, "Containers moved successfully!")
	}
	return nil
}

func rebalanceContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	dry := false
	params, err := unmarshal(r.Body)
	if err == nil {
		dry = params["dry"] == "true"
	}
	encoder := json.NewEncoder(w)
	err = rebalanceContainers(encoder, dry)
	if err != nil {
		logProgress(encoder, "Error trying to rebalance containers: %s", err.Error())
	} else {
		logProgress(encoder, "Containers rebalanced successfully!")
	}
	return nil
}

//listContainersHandler call scheduler.Containers to list all containers into it.
func listContainersHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	address := r.URL.Query().Get(":address")
	if address != "" {
		containerList, err := listContainersByHost(address)
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(containerList)
	}
	app := r.URL.Query().Get(":appname")
	containerList, err := listContainersByApp(app)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(containerList)
}

func addPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	var segScheduler segregatedScheduler
	return segScheduler.addPool(params["pool"])
}

func removePoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	params, err := unmarshal(r.Body)
	if err != nil {
		return err
	}
	var segScheduler segregatedScheduler
	return segScheduler.removePool(params["pool"])
}

func listPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var pools []Pool
	err = conn.Collection(schedulerCollection).Find(nil).All(&pools)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(pools)
}

type teamsToPoolParams struct {
	Pool  string   `json:"pool"`
	Teams []string `json:"teams"`
}

func addTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	var segScheduler segregatedScheduler
	return segScheduler.addTeamsToPool(params.Pool, params.Teams)
}

func removeTeamToPoolHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var params teamsToPoolParams
	err = json.Unmarshal(b, &params)
	if err != nil {
		return err
	}
	var segScheduler segregatedScheduler
	return segScheduler.removeTeamsFromPool(params.Pool, params.Teams)
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

func sshToContainerHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	containerID := r.URL.Query().Get(":container_id")
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	height, _ := strconv.Atoi(r.URL.Query().Get("height"))
	container, err := getContainer(containerID)
	if err != nil {
		return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: "cannot hijack connection",
		}
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return &errors.HTTP{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		}
	}
	defer conn.Close()
	return container.shell(conn, conn, conn, pty{width: width, height: height})
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
