package service

import (
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/database"
)

type ServiceType struct {
	Id    int64
	Name  string
	Charm string
}

func (st *ServiceType) Get() error {
	query := make(map[string]interface{})

	switch {
	case st.Id != 0:
		query["id"] = st.Id
	case st.Name != "":
		query["name"] = st.Name
	case st.Charm != "":
		query["charm"] = st.Charm
	}

	c := Mdb.C("service_types")
	err := c.Find(query).One(&st)
	if err != nil {
		panic(err)
	}

	return nil
}

func (s *ServiceType) All() (result []ServiceType) {
	result = make([]ServiceType, 0)

	c := Mdb.C("service_types")
	iter := c.Find(nil).Limit(100).Iter()
	err := iter.All(&result)

	if err != nil {
		panic(err)
	}

	return
}

func (st *ServiceType) Create() error {
	c := Mdb.C("service_types")
	err := c.Insert(st)
	if err != nil {
		panic(err)
	}

	return err
}

func (st *ServiceType) Delete() error {
	c := Mdb.C("service_types")
	err := c.Remove(st) // should pass specific fields instead using all them
	if err != nil {
		panic(err)
	}

	return nil
}
