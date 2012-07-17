package app_test

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	"reflect"
	"testing"
)

func TestAppIsABinderApp(t *testing.T) {
	var a bind.App
	iface := reflect.ValueOf(&a)
	obj := reflect.ValueOf(&app.App{})
	if !obj.Type().Implements(iface.Elem().Type()) {
		t.Errorf("app.App should implement bind.App")
	}
}

func TestDestroyShouldUnbindAppFromInstance(t *testing.T) {
	db.Session, _ = db.Open("127.0.0.1:27017", "tsuru_app_bind_test")
	defer func() {
		db.Session.Apps().Database.DropDatabase()
		db.Session.Close()
	}()
	instance := service.ServiceInstance{
		Name: "MyInstance",
		Apps: []string{"myApp"},
	}
	err := instance.Create()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer db.Session.ServiceInstances().Remove(bson.M{"_id": instance.Name})
	a := app.App{
		Name: "myApp",
	}
	err = a.Create()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	defer db.Session.Apps().Remove(bson.M{"name": a.Name})
	err = a.Destroy()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	n, _ := db.Session.ServiceInstances().Find(bson.M{"apps": bson.M{"$in": []string{a.Name}}}).Count()
	if n != 0 {
		t.Errorf("Should unbind apps when destroying them, but did not.\nExpected 0 apps to be binded to the instance %s, got %d.", instance.Name, n)
	}
}
