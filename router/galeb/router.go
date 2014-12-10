// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/router"
	galebClient "github.com/tsuru/tsuru/router/galeb/client"
	"gopkg.in/mgo.v2/bson"
)

const routerName = "galeb"

type galebRouter struct {
	client *galebClient.GalebClient
}

type galebCNameData struct {
	CName         string
	VirtualHostId string
}

type galebRealData struct {
	Real      string
	BackendId string
}

type galebData struct {
	Name          string `bson:"_id"`
	BackendPoolId string
	RootRuleId    string
	VirtualHostId string
	CNames        []galebCNameData
	Reals         []galebRealData
}

func (g *galebData) save() error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.Insert(g)
}

func (g *galebData) addReal(address, backendId string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.UpdateId(g.Name, bson.M{"$push": bson.M{
		"reals": bson.M{"real": address, "backendid": backendId},
	}})
}

func (g *galebData) removeReal(address string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.UpdateId(g.Name, bson.M{"$pull": bson.M{
		"reals": bson.M{"real": address},
	}})
}

func (g *galebData) addCName(cname, virtualHostId string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.UpdateId(g.Name, bson.M{"$push": bson.M{
		"cnames": bson.M{"cname": cname, "virtualhostid": virtualHostId},
	}})
}

func (g *galebData) removeCName(cname string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.UpdateId(g.Name, bson.M{"$pull": bson.M{
		"cnames": bson.M{"cname": cname},
	}})
}

func (g *galebData) remove() error {
	coll, err := collection()
	if err != nil {
		return err
	}
	return coll.RemoveId(g.Name)
}

func getGalebData(name string) (*galebData, error) {
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	var result galebData
	err = coll.Find(bson.M{"_id": name}).One(&result)
	return &result, err
}

func init() {
	router.Register(routerName, &galebRouter{})
}

func collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("galeb_router"), nil
}

func poolName(base string) string {
	return fmt.Sprintf("tsuru-backendpool-%s", base)
}

func rootRuleName(base string) string {
	return fmt.Sprintf("tsuru-rootrule-%s", base)
}

func virtualHostName(base string) string {
	domain, _ := config.GetString("galeb:domain")
	return fmt.Sprintf("%s.%s", base, domain)
}

func (r *galebRouter) getClient() (*galebClient.GalebClient, error) {
	if r.client == nil {
		var err error
		r.client, err = galebClient.NewGalebClient()
		if err != nil {
			return nil, err
		}
	}
	return r.client, nil
}

func (r *galebRouter) AddBackend(name string) error {
	poolParams := galebClient.BackendPoolParams{
		Name: poolName(name),
	}
	data := galebData{Name: name}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	data.BackendPoolId, err = client.AddBackendPool(&poolParams)
	if err != nil {
		return err
	}
	ruleParams := galebClient.RuleParams{
		Name:        rootRuleName(name),
		Match:       "/",
		BackendPool: data.BackendPoolId,
	}
	data.RootRuleId, err = client.AddRule(&ruleParams)
	if err != nil {
		return err
	}
	virtualHostParams := galebClient.VirtualHostParams{
		Name:        virtualHostName(name),
		RuleDefault: data.RootRuleId,
	}
	data.VirtualHostId, err = client.AddVirtualHost(&virtualHostParams)
	if err != nil {
		return err
	}
	err = data.save()
	if err != nil {
		return err
	}
	return router.Store(name, name, routerName)
}

func (r *galebRouter) RemoveBackend(name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return err
	}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	err = client.RemoveResource(data.VirtualHostId)
	if err != nil {
		return err
	}
	for _, cnameData := range data.CNames {
		err = client.RemoveResource(cnameData.VirtualHostId)
		if err != nil {
			return err
		}
	}
	err = client.RemoveResource(data.RootRuleId)
	if err != nil {
		return err
	}
	err = client.RemoveResource(data.BackendPoolId)
	if err != nil {
		return err
	}
	err = data.remove()
	if err != nil {
		return err
	}
	return router.Remove(backendName)
}

func (r *galebRouter) AddRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return err
	}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(address)
	if parsed != nil && parsed.Host != "" {
		address = parsed.Host
	}
	host, portStr, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portStr)
	params := galebClient.BackendParams{
		Ip:          host,
		Port:        port,
		BackendPool: data.BackendPoolId,
	}
	backendId, err := client.AddBackend(&params)
	if err != nil {
		return err
	}
	return data.addReal(address, backendId)
}

func (r *galebRouter) RemoveRoute(name, address string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return err
	}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(address)
	if parsed != nil && parsed.Host != "" {
		address = parsed.Host
	}
	for _, real := range data.Reals {
		if real.Real == address {
			err = client.RemoveResource(real.BackendId)
			if err != nil {
				return err
			}
			break
		}
	}
	return data.removeReal(address)
}

func (r *galebRouter) SetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return err
	}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	virtualHostParams := galebClient.VirtualHostParams{
		Name:        cname,
		RuleDefault: data.RootRuleId,
	}
	virtualHostId, err := client.AddVirtualHost(&virtualHostParams)
	if err != nil {
		return err
	}
	return data.addCName(cname, virtualHostId)
}

func (r *galebRouter) UnsetCName(cname, name string) error {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return err
	}
	client, err := r.getClient()
	if err != nil {
		return err
	}
	for _, cnameData := range data.CNames {
		if cnameData.CName == cname {
			err = client.RemoveResource(cnameData.VirtualHostId)
			if err != nil {
				return err
			}
			break
		}
	}
	return data.removeCName(cname)
}

func (r *galebRouter) Addr(name string) (string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return "", err
	}
	_, err = getGalebData(backendName)
	if err != nil {
		return "", err
	}
	return virtualHostName(backendName), nil
}

func (r *galebRouter) Swap(backend1, backend2 string) error {
	return router.Swap(r, backend1, backend2)
}

func (r *galebRouter) Routes(name string) ([]string, error) {
	backendName, err := router.Retrieve(name)
	if err != nil {
		return nil, err
	}
	data, err := getGalebData(backendName)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, real := range data.Reals {
		hosts = append(hosts, real.Real)
	}
	return hosts, nil
}
