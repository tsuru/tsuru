package app_test

import (
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/bind"
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
