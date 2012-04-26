package service

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
)

type Service struct {
	Id            bson.ObjectId `bson:"_id"`
	ServiceTypeId bson.ObjectId `bson:"service_type_id"`
	Name          string
}

func (s *Service) Get() error {
	query := bson.M{}
	switch {
	case s.Id != "":
		query["_id"] = s.Id
	case s.Name != "":
		query["name"] = s.Name
	}
	return db.Session.Services().Find(query).One(&s)
}

func (s *Service) All() []Service {
	var result []Service
	db.Session.Services().Find(nil).All(&result)
	return result
}

func (s *Service) Create() error {
	s.Id = bson.NewObjectId()
	err := db.Session.Services().Insert(s)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	return u.Create()
}

func (s *Service) Delete() error {
	err := db.Session.Services().Remove(s)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Charm}
	return u.Destroy()
}

func (s *Service) Bind(a *app.App) error {
	sa := ServiceApp{ServiceId: s.Id, AppName: a.Name}
	return sa.Create()
}

func (s *Service) Unbind(a *app.App) error {
	sa := ServiceApp{ServiceId: s.Id, AppName: a.Name}
	return sa.Delete()
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}
