package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

func (s *S) TestAddAutoScaleUnits(c *check.C) {
	s.mockService.AppQuota.OnGet = func(item quota.QuotaItem) (*quota.Quota, error) {
		c.Assert(item.GetName(), check.Equals, "myapp")
		return &quota.Quota{
			Limit: 10,
		}, nil
	}
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("autoscaleProv")
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	b := strings.NewReader(`{"process": "p1", "minUnits": 2, "maxUnits": 10, "averageCPU": "600m"}`)
	request, err := http.NewRequest("POST", "/apps/myapp/units/autoscale", b)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	spec, err := a.AutoScaleInfo(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(spec, check.DeepEquals, []provision.AutoScaleSpec{
		{Process: "p1", MinUnits: 2, MaxUnits: 10, AverageCPU: "600m"},
	})
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update.unit.autoscale.add",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "averageCPU", "value": "600m"},
			{"name": "process", "value": "p1"},
			{"name": "minUnits", "value": "2"},
			{"name": "maxUnits", "value": "10"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestRemoveAutoScaleUnits(c *check.C) {
	provision.DefaultProvisioner = "autoscaleProv"
	provision.Register("autoscaleProv", func() (provision.Provisioner, error) {
		return &provisiontest.AutoScaleProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}, nil
	})
	defer provision.Unregister("autoscaleProv")
	a := app.App{Name: "myapp", Platform: "zend", TeamOwner: s.team.Name}
	err := app.CreateApp(context.TODO(), &a, s.user)
	c.Assert(err, check.IsNil)
	err = a.AutoScale(context.TODO(), provision.AutoScaleSpec{
		Process:    "p1",
		AverageCPU: "300m",
		MaxUnits:   10,
		MinUnits:   2,
	})
	c.Assert(err, check.IsNil)
	token := userWithPermission(c, permission.Permission{
		Scheme:  permission.PermAppUpdate,
		Context: permission.Context(permTypes.CtxApp, a.Name),
	})
	request, err := http.NewRequest("DELETE", "/apps/myapp/units/autoscale?process=p1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+token.GetValue())
	recorder := httptest.NewRecorder()
	s.testServer.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	spec, err := a.AutoScaleInfo(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(spec, check.IsNil)
	c.Assert(eventtest.EventDesc{
		Target: appTarget("myapp"),
		Owner:  token.GetUserName(),
		Kind:   "app.update.unit.autoscale.remove",
		StartCustomData: []map[string]interface{}{
			{"name": ":app", "value": a.Name},
			{"name": "process", "value": "p1"},
		},
	}, eventtest.HasEvent)
}
