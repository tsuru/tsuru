// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/types/router"
	check "gopkg.in/check.v1"
)

func (s *S) TestTemplateServiceSave(c *check.C) {
	svc, err := TemplateService()
	c.Assert(err, check.IsNil)
	err = svc.Save(router.RouterTemplate{})
	c.Assert(err, check.ErrorMatches, `router template name and type are required`)
	err = svc.Save(router.RouterTemplate{
		Name: "myrouter",
		Type: "invalid",
	})
	c.Assert(err, check.ErrorMatches, `router type "invalid" is not registered`)
	config.Set("routers:mine:type", "myrouter")
	defer config.Unset("routers:mine")
	Register("myrouter", func(name string, config ConfigGetter) (Router, error) {
		return nil, nil
	})
	err = svc.Save(router.RouterTemplate{
		Name: "mine",
		Type: "myrouter",
	})
	c.Assert(err, check.ErrorMatches, `router named "mine" already exists in config`)
	err = svc.Save(router.RouterTemplate{
		Name: "mine2",
		Type: "myrouter",
	})
	c.Assert(err, check.IsNil)
}
