package service

import (
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
)

type ServiceType struct {
	Name  string `bson:"_id"`
	Charm string
}

func (st *ServiceType) Get() error {
	query := bson.M{}

	switch {
	case st.Name != "":
		query["_id"] = st.Name
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
	return db.Session.ServiceTypes().Insert(st)
}

func (st *ServiceType) Delete() error {
	return db.Session.ServiceTypes().Remove(st)
}
