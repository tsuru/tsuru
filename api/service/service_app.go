package service

import (
	_ "github.com/mattn/go-sqlite3"
	. "github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
	"github.com/timeredbull/tsuru/api/unit"
)

type ServiceApp struct {
	Id        int64
	ServiceId int64
	AppId     int64
}

func (sa *ServiceApp) Create() error {
	c := Mdb.C("service_apps")
	err := c.Insert(sa)
	if err != nil {
		panic(err)
	}

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.AddRelation(&serviceUnit)

	return err
}

func (sa *ServiceApp) Delete() error {
	c := Mdb.C("service_apps")
	err := c.Remove(sa) // should pass specific fields instead using all them
	if err != nil {
		panic(err)
	}

	s := sa.Service()
	a := sa.App()

	appUnit := unit.Unit{Name: a.Name}
	serviceUnit := unit.Unit{Name: s.Name}
	appUnit.RemoveRelation(&serviceUnit)

	return nil
}

func (sa *ServiceApp) Service() (s *Service) {
	s = &Service{Id: sa.ServiceId}
	s.Get()

	return
}

func (sa *ServiceApp) App() (a *App) {
	query := make(map[string]interface{})
	query["id"] = sa.AppId

	c := Mdb.C("apps")
	c.Find(query).One(&a)
	// query := "SELECT id, name, framework FROM apps WHERE id = ?"
	// rows, err := Db.Query(query, sa.AppId)
	// if err != nil {
	// 	panic(err)
	// }

	// var id int64
	// var name string
	// var framework string
	// for rows.Next() {
	// 	rows.Scan(&id, &name, &framework)
	// }

	// a = &App{
	// 	Id:        id,
	// 	Name:      name,
	// 	Framework: framework,
	// }

	return
}
