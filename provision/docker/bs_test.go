// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"runtime"
	"sync"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestGetImageFromDatabase(c *check.C) {
	imageName := "tsuru/bsss"
	coll, err := bsCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = coll.Insert(bsConfig{ID: bsUniqueID, Image: imageName})
	c.Assert(err, check.IsNil)
	defer coll.Remove(bson.M{"image": imageName})
	conf, err := loadBsConfig()
	c.Assert(err, check.IsNil)
	image := conf.getImage()
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageFromConfig(c *check.C) {
	imageName := "tsuru/bs:v10"
	config.Set("docker:bs:image", imageName)
	conf := bsConfig{}
	image := conf.getImage()
	c.Assert(image, check.Equals, imageName)
}

func (s *S) TestGetImageDefaultValue(c *check.C) {
	config.Unset("docker:bs:image")
	conf := bsConfig{}
	image := conf.getImage()
	c.Assert(image, check.Equals, "tsuru/bs")
}

func (s *S) TestSaveImage(c *check.C) {
	coll, err := bsCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	err = saveBsImage("tsuru/bs@sha1:afd533420cf")
	c.Assert(err, check.IsNil)
	var configs []bsConfig
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420cf")
	err = saveBsImage("tsuru/bs@sha1:afd533420d0")
	c.Assert(err, check.IsNil)
	err = coll.Find(nil).All(&configs)
	c.Assert(err, check.IsNil)
	c.Assert(configs, check.HasLen, 1)
	c.Assert(configs[0].Image, check.Equals, "tsuru/bs@sha1:afd533420d0")
}

func (s *S) TestBsGetToken(c *check.C) {
	conf := bsConfig{}
	token, err := conf.getToken()
	c.Assert(err, check.IsNil)
	c.Assert(token, check.Equals, conf.Token)
	c.Assert(token, check.Not(check.Equals), "")
	token2, err := conf.getToken()
	c.Assert(token2, check.Equals, token)
}

func (s *S) TestBsGetTokenStress(c *check.C) {
	runtime.GOMAXPROCS(runtime.GOMAXPROCS(10))
	var tokens []string
	var mutex sync.Mutex
	var wg sync.WaitGroup
	getToken := func(wg *sync.WaitGroup) {
		defer wg.Done()
		conf := bsConfig{}
		t, err := conf.getToken()
		c.Assert(err, check.IsNil)
		mutex.Lock()
		tokens = append(tokens, t)
		mutex.Unlock()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go getToken(&wg)
	}
	wg.Wait()
	for i := 1; i < len(tokens); i++ {
		c.Assert(tokens[i-1], check.Equals, tokens[i])
	}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	n, err := conn.Tokens().Find(bson.M{"appname": app.InternalAppName}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
	coll, err := bsCollection()
	c.Assert(err, check.IsNil)
	defer coll.Close()
	n, err = coll.Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
}
