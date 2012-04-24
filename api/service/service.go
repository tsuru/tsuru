package service

import (
	. "github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/unit"
	. "github.com/timeredbull/tsuru/database"
	"launchpad.net/mgo/bson"
)

type Service struct {
	Id            bson.ObjectId "_id"
	ServiceTypeId bson.ObjectId "service_type_id"
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
	c := Db.C("services")
	return c.Find(query).One(&s)
}

func (s *Service) All() (result []Service) {
	c := Db.C("services")
	c.Find(nil).All(&result)
	return
}

func (s *Service) Create() error {
	c := Db.C("services")
	s.Id = bson.NewObjectId()
	err := c.Insert(s)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	return u.Create()
}

func (s *Service) Delete() error {
	c := Db.C("services")
	err := c.Remove(s)
	if err != nil {
		return err
	}

	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Charm}
	return u.Destroy()
}

func (s *Service) Bind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	return sa.Create()
}

func (s *Service) Unbind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	return sa.Delete()
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}
