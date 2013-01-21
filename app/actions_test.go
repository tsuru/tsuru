// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"github.com/fsouza/go-iam"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/queue"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/s3"
	. "launchpad.net/gocheck"
	"strings"
)

func (s *S) TestInsertAppForward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestInsertAppForwardAppPointer(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(a.Framework, Equals, app.Framework)
	err = app.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *C) {
	ctx := action.FWContext{
		Params: []interface{}{"hello"},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "First parameter must be App or *App.")
}

func (s *S) TestInsertAppBackward(c *C) {
	app := App{Name: "conviction", Framework: "evergrey"}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name}) // sanity
	insertApp.Backward(ctx)
	n, err := s.conn.Apps().Find(bson.M{"name": app.Name}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 0)
}

func (s *S) TestInsertAppMinimumParams(c *C) {
	c.Assert(insertApp.MinParams, Equals, 1)
}

func (s *S) TestCreateIAMUserForward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	app := App{Name: "trapped"}
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &app}
	result, err := createIAMUserAction.Forward(ctx)
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(app.Name)
	u, ok := result.(*iam.User)
	c.Assert(ok, Equals, true)
	c.Assert(u.Name, Equals, app.Name)
}

func (s *S) TestCreateIAMUserBackward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	app := App{Name: "escape"}
	user, err := createIAMUser(app.Name)
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(user.Name)
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: user}
	createIAMUserAction.Backward(ctx)
	_, err = iamClient.GetUser(user.Name)
	c.Assert(err, NotNil)
}

func (s *S) TestCreateIAMUserMinParams(c *C) {
	c.Assert(createIAMUserAction.MinParams, Equals, 1)
}

func (s *S) TestCreateIAMAccessKeyForward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("puppets", "/")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	ctx := action.FWContext{Params: []interface{}{nil}, Previous: &resp.User}
	result, err := createIAMAccessKeyAction.Forward(ctx)
	c.Assert(err, IsNil)
	ak, ok := result.(*iam.AccessKey)
	c.Assert(ok, Equals, true)
	c.Assert(ak.UserName, Equals, resp.User.Name)
	c.Assert(ak.Id, Not(Equals), "")
	c.Assert(ak.Secret, Not(Equals), "")
	defer iamClient.DeleteAccessKey(ak.Id, ak.UserName)
}

func (s *S) TestCreateIAMAccessKeyBackward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("myuser", "/")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	kresp, err := iamClient.CreateAccessKey(resp.User.Name)
	c.Assert(err, IsNil)
	defer iamClient.DeleteAccessKey(kresp.AccessKey.Id, resp.User.Name)
	ctx := action.BWContext{Params: []interface{}{nil}, FWResult: &kresp.AccessKey}
	createIAMAccessKeyAction.Backward(ctx)
	akResp, err := iamClient.AccessKeys(resp.User.Name)
	c.Assert(err, IsNil)
	c.Assert(akResp.AccessKeys, HasLen, 0)
}

func (s *S) TestCreateIAMMinParams(c *C) {
	c.Assert(createIAMAccessKeyAction.MinParams, Equals, 1)
}

func (s *S) TestCreateBucketForward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{S3Endpoint: s.t.S3Server.URL()}
	s3Client := s3.New(auth, region)
	app := App{Name: "leper"}
	ctx := action.FWContext{
		Params:   []interface{}{&app},
		Previous: &iam.AccessKey{Id: "access", Secret: "s3cr3t"},
	}
	result, err := createBucketAction.Forward(ctx)
	c.Assert(err, IsNil)
	env, ok := result.(*s3Env)
	c.Assert(ok, Equals, true)
	c.Assert(env.AccessKey, Equals, "access")
	c.Assert(env.SecretKey, Equals, "s3cr3t")
	c.Assert(env.endpoint, Equals, s.t.S3Server.URL())
	c.Assert(env.locationConstraint, Equals, true)
	defer s3Client.Bucket(env.bucket).DelBucket()
	_, err = s3Client.Bucket(env.bucket).List("", "/", "", 100)
	c.Assert(err, IsNil)
}

func (s *S) TestCreateBucketBackward(c *C) {
	patchRandomReader()
	defer unpatchRandomReader()
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{
		Name:                 "myregion",
		S3Endpoint:           s.t.S3Server.URL(),
		S3LocationConstraint: true,
		S3LowercaseBucket:    true,
	}
	s3Client := s3.New(auth, region)
	app := App{Name: "leper"}
	err := s3Client.Bucket(app.Name).PutBucket(s3.BucketOwnerFull)
	c.Assert(err, IsNil)
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name,
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: &env}
	createBucketAction.Backward(ctx)
	_, err = s3Client.Bucket(app.Name).List("", "/", "", 100)
	c.Assert(err, NotNil)
}

func (s *S) TestCreateBucketMinParams(c *C) {
	c.Assert(createBucketAction.MinParams, Equals, 1)
}

func (s *S) TestCreateUserPolicyForward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("blackened", "/")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	app := App{Name: resp.User.Name}
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name,
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &env}
	result, err := createUserPolicyAction.Forward(ctx)
	c.Assert(err, IsNil)
	e, ok := result.(*s3Env)
	c.Assert(ok, Equals, true)
	c.Assert(e, Equals, &env)
	_, err = iamClient.GetUserPolicy(resp.User.Name, "app-blackened-bucket")
	c.Assert(err, IsNil)
}

func (s *S) TestCreateUserPolicyBackward(c *C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("blackened", "/")
	c.Assert(err, IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	app := App{Name: resp.User.Name}
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name,
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	_, err = iamClient.PutUserPolicy(resp.User.Name, "app-blackened-bucket", "null")
	c.Assert(err, IsNil)
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: &env}
	createUserPolicyAction.Backward(ctx)
	_, err = iamClient.GetUserPolicy(resp.User.Name, "app-blackened-bucket")
	c.Assert(err, NotNil)
}

func (s *S) TestCreateUsePolicyMinParams(c *C) {
	c.Assert(createUserPolicyAction.MinParams, Equals, 1)
}

func (s *S) TestExportEnvironmentsForward(c *C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "mist", Framework: "opeth"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name + "-bucket",
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &env}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, &env)
	err = app.Get()
	c.Assert(err, IsNil)
	appEnv := app.InstanceEnv(s3InstanceName)
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Value, Equals, env.endpoint)
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Value, Equals, "true")
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_ACCESS_KEY_ID"].Value, Equals, env.AccessKey)
	c.Assert(appEnv["TSURU_S3_ACCESS_KEY_ID"].Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_SECRET_KEY"].Value, Equals, env.SecretKey)
	c.Assert(appEnv["TSURU_S3_SECRET_KEY"].Public, Equals, false)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Value, Equals, env.bucket)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Public, Equals, false)
	appEnv = app.InstanceEnv("")
	c.Assert(appEnv["TSURU_APPNAME"].Value, Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, Equals, false)
	message, err := queue.Get(queueName, 2e9)
	c.Assert(err, IsNil)
	defer message.Delete()
	c.Assert(message.Action, Equals, regenerateApprc)
	c.Assert(message.Args, DeepEquals, []string{app.Name})
}

func (s *S) TestExportEnvironmentsForwardWithoutS3Env(c *C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "mist", Framework: "opeth"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &app}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, Equals, &app)
	err = app.Get()
	c.Assert(err, IsNil)
	appEnv := app.InstanceEnv(s3InstanceName)
	c.Assert(appEnv, DeepEquals, map[string]bind.EnvVar{})
	appEnv = app.InstanceEnv("")
	c.Assert(appEnv["TSURU_APPNAME"].Value, Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, Equals, false)
}

func (s *S) TestExportEnvironmentsBackward(c *C) {
	envNames := []string{
		"TSURU_S3_ACCESS_KEY_ID", "TSURU_S3_SECRET_KEY",
		"TSURU_APPNAME", "TSURU_HOST", "TSURU_S3_ENDPOINT",
		"TSURU_S3_LOCATIONCONSTRAINT", "TSURU_S3_BUCKET",
	}
	app := App{Name: "moon", Framework: "opeth", Env: make(map[string]bind.EnvVar)}
	for _, name := range envNames {
		envVar := bind.EnvVar{Name: name, Value: name, Public: false}
		if strings.HasPrefix(name, "TSURU_S3_") {
			envVar.InstanceName = s3InstanceName
		}
		app.Env[name] = envVar
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.BWContext{Params: []interface{}{&app}}
	exportEnvironmentsAction.Backward(ctx)
	copy := app
	err = copy.Get()
	c.Assert(err, IsNil)
	for _, name := range envNames {
		if _, ok := copy.Env[name]; ok {
			c.Errorf("Variable %q should be unexported, but it's still exported.", name)
		}
	}
}

func (s *S) TestExportEnvironmentsMinParams(c *C) {
	c.Assert(exportEnvironmentsAction.MinParams, Equals, 1)
}

func (s *S) TestCreateRepositoryForward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestCreateRepositoryForwardAppPointer(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	c.Assert(err, IsNil)
	c.Assert(h.url[0], Equals, "/repository")
	c.Assert(h.method[0], Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), Equals, expected)
}

func (s *S) TestCreateRepositoryForwardInvalidType(c *C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := createRepository.Forward(ctx)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "First parameter must be App or *App.")
}

func (s *S) TestCreateRepositoryBackward(c *C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp"}
	ctx := action.BWContext{FWResult: &app, Params: []interface{}{app}}
	createRepository.Backward(ctx)
	c.Assert(h.url[0], Equals, "/repository/someapp")
	c.Assert(h.method[0], Equals, "DELETE")
	c.Assert(string(h.body[0]), Equals, "null")
}

func (s *S) TestCreateRepositoryMinParams(c *C) {
	c.Assert(createRepository.MinParams, Equals, 1)
}

func (s *S) TestProvisionAppForward(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, IsNil)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, 0)
}

func (s *S) TestProvisionAppForwardAppPointer(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, IsNil)
	a, ok := result.(*App)
	c.Assert(ok, Equals, true)
	c.Assert(a.Name, Equals, app.Name)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, 0)
}

func (s *S) TestProvisionAppForwardInvalidApp(c *C) {
	ctx := action.FWContext{Params: []interface{}{"something", 1}}
	_, err := provisionApp.Forward(ctx)
	c.Assert(err, NotNil)
}

func (s *S) TestProvisionAppBackward(c *C) {
	app := App{
		Name:      "earthshine",
		Framework: "django",
		Units:     []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	fwctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(fwctx)
	c.Assert(err, IsNil)
	bwctx := action.BWContext{Params: []interface{}{&app, 4}, FWResult: result}
	provisionApp.Backward(bwctx)
	index := s.provisioner.FindApp(&app)
	c.Assert(index, Equals, -1)
}

func (s *S) TestProvisionAppMinParams(c *C) {
	c.Assert(provisionApp.MinParams, Equals, 2)
}

func (s *S) TestProvisionAddUnitsForward(c *C) {
	app := App{
		Name:      "castle",
		Framework: "heavens",
		Units:     []Unit{{Machine: 2}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	previous, err := provisionApp.Forward(action.FWContext{Params: []interface{}{&app, 4}})
	c.Assert(err, IsNil)
	defer provisionApp.Backward(action.BWContext{Params: []interface{}{&app, 4}, FWResult: previous})
	ctx := action.FWContext{Params: []interface{}{&app, 4}, Previous: previous}
	result, err := provisionAddUnits.Forward(ctx)
	c.Assert(err, IsNil)
	c.Assert(result, IsNil)
	units := s.provisioner.GetUnits(&app)
	c.Assert(units, HasLen, 4)
}

func (s *S) TestProvisionAddUnitsBackward(c *C) {
	c.Assert(provisionAddUnits.Backward, IsNil)
}

func (s *S) TestProvisionAddUnitsMinParams(c *C) {
	c.Assert(provisionAddUnits.MinParams, Equals, 2)
}
