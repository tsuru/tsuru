package service

import (
	stderrors "errors"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"labix.org/v2/mgo/bson"
	"net/http"
)

type ServiceInstance struct {
	Name        string `bson:"_id"`
	ServiceName string `bson:"service_name"`
	Apps        []string
	Teams       []string
	Instance    string
	Host        string
	PrivateHost string `bson:"private_host"`
	State       string
	Env         map[string]string
}

func (si *ServiceInstance) Create() error {
	if si.State == "" {
		si.State = "creating"
	}
	return db.Session.ServiceInstances().Insert(si)
}

func (si *ServiceInstance) Delete() error {
	doc := bson.M{"_id": si.Name, "apps": si.Apps}
	return db.Session.ServiceInstances().Remove(doc)
}

func (si *ServiceInstance) Service() *Service {
	s := &Service{}
	db.Session.Services().Find(bson.M{"_id": si.ServiceName}).One(s)
	return s
}

func (si *ServiceInstance) AddApp(appName string) error {
	index := si.FindApp(appName)
	if index > -1 {
		return stderrors.New("This instance already has this app.")
	}
	si.Apps = append(si.Apps, appName)
	return nil
}

func (si *ServiceInstance) FindApp(appName string) int {
	index := -1
	for i, name := range si.Apps {
		if name == appName {
			index = i
			break
		}
	}
	return index
}

func (si *ServiceInstance) RemoveApp(appName string) error {
	index := si.FindApp(appName)
	if index < 0 {
		return stderrors.New("This app is not binded to this service instance.")
	}
	last := len(si.Apps) - 1
	if index != last {
		si.Apps[index] = si.Apps[last]
	}
	si.Apps = si.Apps[:last]
	return nil
}

func (si *ServiceInstance) update() error {
	return db.Session.ServiceInstances().Update(bson.M{"_id": si.Name}, si)
}

func (si *ServiceInstance) Bind(app bind.App, user *auth.User) error {
	si.AddApp(app.GetName())
	err := si.update()
	if err != nil {
		return err
	}
	var envVars []bind.EnvVar
	var setEnv = func(env map[string]string) {
		for k, v := range env {
			envVars = append(envVars, bind.EnvVar{
				Name:         k,
				Value:        v,
				Public:       false,
				InstanceName: si.Name,
			})
		}
	}
	setEnv(si.Env)
	var cli *Client
	if cli, err = si.Service().GetClient("production"); err == nil {
		if len(app.GetUnits()) == 0 {
			return &errors.Http{Code: http.StatusPreconditionFailed, Message: "This app does not have an IP yet."}
		}
		env, err := cli.Bind(si, app)
		if err != nil {
			return err
		}
		setEnv(env)
	}
	return app.SetEnvs(envVars, false)
}

func (si *ServiceInstance) Unbind(app bind.App, user *auth.User) error {
	return nil
}
