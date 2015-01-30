// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/router"
)

const hcOk = "WORKING"

type healthchecker interface {
	Healthcheck() error
}

func healthcheck(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") == "all" {
		fullHealthcheck(w, r)
		return
	}
	w.Write([]byte(hcOk))
}

func fullHealthcheck(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	status := http.StatusOK
	mongoDBStatus := hcOk
	fmt.Fprint(&buf, "MongoDB: ")
	conn, err := db.Conn()
	if err != nil {
		status = http.StatusInternalServerError
		mongoDBStatus = fmt.Sprintf("failed to connect - %s", err)
	} else {
		defer conn.Close()
		err = conn.Apps().Database.Session.Ping()
		if err != nil {
			status = http.StatusInternalServerError
			mongoDBStatus = fmt.Sprintf("failed to ping - %s", err)
		}
	}
	fmt.Fprintln(&buf, mongoDBStatus)
	server, err := repository.ServerURL()
	if err != nil && err != repository.ErrGandalfDisabled {
		status = http.StatusInternalServerError
		fmt.Fprintf(&buf, "Gandalf: %s\n", err)
	} else if err == nil {
		gandalfStatus := hcOk
		fmt.Fprint(&buf, "Gandalf: ")
		c := gandalf.Client{Endpoint: server}
		_, err = c.GetHealthCheck()
		if err != nil {
			gandalfStatus = fmt.Sprintf("%s", err)
			status = http.StatusInternalServerError
		}
		fmt.Fprintln(&buf, gandalfStatus)
	}
	if routers, err := config.Get("routers"); err == nil {
		if routersMap, ok := routers.(map[string]interface{}); ok {
			for routerName := range routersMap {
				r, _ := router.Get(routerName)
				if hrouter, ok := r.(healthchecker); ok {
					fmt.Fprintf(&buf, "Router %q: ", routerName)
					routerStatus := hcOk
					if err := hrouter.Healthcheck(); err != nil {
						status = http.StatusInternalServerError
						routerStatus = fmt.Sprintf("fail - %s", err)
					}
					fmt.Fprintln(&buf, routerStatus)
				}
			}
		}
	}
	if hprovisioner, ok := app.Provisioner.(healthchecker); ok {
		fmt.Fprint(&buf, "Provisioner: ")
		provisionerStatus := hcOk
		if err := hprovisioner.Healthcheck(); err != nil {
			status = http.StatusInternalServerError
			provisionerStatus = fmt.Sprintf("fail - %s", err)
		}
		fmt.Fprintln(&buf, provisionerStatus)
	}
	if iaases, err := config.Get("iaas"); err == nil {
		if iaasesMap, ok := iaases.(map[string]interface{}); ok {
			var names []string
			for iaasname := range iaasesMap {
				switch iaasname {
				case "default":
					names = append(names, iaasesMap[iaasname].(string))
				case "custom":
					customIaas, ok := iaasesMap[iaasname].(map[string]interface{})
					if ok {
						for name := range customIaas {
							names = append(names, name)
						}
					}
				default:
					names = append(names, iaasname)
				}
			}
			for _, name := range names {
				provider, _ := iaas.GetIaasProvider(name)
				if hprovider, ok := provider.(healthchecker); ok {
					fmt.Fprintf(&buf, "IaaS %q: ", name)
					iaasStatus := hcOk
					if err := hprovider.Healthcheck(); err != nil {
						status = http.StatusInternalServerError
						iaasStatus = fmt.Sprintf("fail - %s", err)
					}
					fmt.Fprintln(&buf, iaaStatus)
				}
			}
		}
	}
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
