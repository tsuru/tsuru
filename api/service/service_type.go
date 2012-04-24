package service

import (
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/database"
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

	c := Db.C("service_types")
	return c.Find(query).One(&st)
}

func (s *ServiceType) All() (result []ServiceType) {
	c := Db.C("service_types")
	c.Find(nil).All(&result)
	return
}

func (st *ServiceType) Create() error {
	c := Db.C("service_types")
	st.Id = bson.NewObjectId()
	return c.Insert(st)
}

func (st *ServiceType) Delete() error {
	c := Db.C("service_types")
	return c.Remove(st) // should pass specific fields instead using all them
}
