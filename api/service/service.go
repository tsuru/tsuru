package service

import (
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	"github.com/timeredbull/tsuru/api/unit"
	"launchpad.net/mgo/bson"
)

type Service struct {
	Id            bson.ObjectId "_id"
	ServiceTypeId bson.ObjectId
	Name          string
}

func (s *Service) Get() error {
	query := make(map[string]interface{})
	var err error
	switch {
	case s.Id != "":
		query["id"] = s.Id
	case s.Name != "":
		query["name"] = s.Name
	}

	c := Mdb.C("services")
	err = c.Find(query).One(&s)

	if err != nil {
		panic(err)
	}

	return nil
}

func (s *Service) All() (result []Service) {
	result = make([]Service, 100)

	c := Mdb.C("services")
	iter := c.Find(nil).Limit(100).Iter()
	err := iter.All(&result)
	if err != nil {
		panic(iter.Err())
	}

	return
}

func (s *Service) Create() error {
	c := Mdb.C("services")
	s.Id = bson.NewObjectId()
	doc := bson.M{"name": s.Name, "service_type_id": s.ServiceTypeId, "_id": s.Id}
	err := c.Insert(doc)

	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: s.Name, Type: "mysql"}
	err = u.Create()

	return err
}

func (s *Service) Delete() error {
	c := Mdb.C("services")
	doc := bson.M{"name": s.Name, "service_type_id": s.ServiceTypeId}
	err := c.Remove(doc) // should pass specific fields instead using all them

	if err != nil {
		panic(err)
	}

	u := unit.Unit{Name: s.Name, Type: s.ServiceType().Charm}
	err = u.Destroy()

	return nil
}

func (s *Service) Bind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Create()

	return nil
}

func (s *Service) Unbind(app *App) error {
	sa := ServiceApp{ServiceId: s.Id, AppId: app.Id}
	sa.Delete()

	return nil
}

func (s *Service) ServiceType() (st *ServiceType) {
	st = &ServiceType{Id: s.ServiceTypeId}
	st.Get()
	return
}
