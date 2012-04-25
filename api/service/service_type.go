package service

import (
	"github.com/timeredbull/tsuru/db"
	"launchpad.net/mgo/bson"
)

type ServiceType struct {
	Id    bson.ObjectId "_id"
	Name  string
	Charm string
}

func (st *ServiceType) Get() error {
	query := bson.M{}

	switch {
	case st.Id != "":
		query["_id"] = st.Id
	case st.Name != "":
		query["name"] = st.Name
	case st.Charm != "":
		query["charm"] = st.Charm
	}

	return db.Session.ServiceTypes().Find(query).One(&st)
}

func (s *ServiceType) All() []ServiceType {
	var result []ServiceType
	db.Session.ServiceTypes().Find(nil).All(&result)
	return result
}

func (st *ServiceType) Create() error {
	st.Id = bson.NewObjectId()
	return db.Session.ServiceTypes().Insert(st)
}

func (st *ServiceType) Delete() error {
	return db.Session.ServiceTypes().Remove(st)
}
