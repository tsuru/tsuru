// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/action"
	"github.com/globocom/tsuru/app/bind"
	"github.com/globocom/tsuru/auth"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/quota"
	"labix.org/v2/mgo/bson"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/iam"
	"launchpad.net/goamz/s3"
	"launchpad.net/gocheck"
	"sort"
	"strings"
)

func (s *S) TestInsertAppForward(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(a.Platform, gocheck.Equals, app.Platform)
	c.Assert(a.Units, gocheck.HasLen, 1)
	c.Assert(a.Units[0].Name, gocheck.Equals, "")
	c.Assert(a.Units[0].QuotaItem, gocheck.Equals, a.Name+"-0")
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units, gocheck.HasLen, 1)
	c.Assert(app.Units[0].Name, gocheck.Equals, "")
	c.Assert(app.Units[0].QuotaItem, gocheck.Equals, a.Name+"-0")
}

func (s *S) TestInsertAppForwardAppPointer(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	a, ok := r.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(a.Platform, gocheck.Equals, app.Platform)
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestInsertAppForwardInvalidValue(c *gocheck.C) {
	ctx := action.FWContext{
		Params: []interface{}{"hello"},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be App or *App.")
}

func (s *S) TestInsertAppDuplication(c *gocheck.C) {
	app := App{Name: "come", Platform: "gotthard"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{
		Params: []interface{}{&app},
	}
	r, err := insertApp.Forward(ctx)
	c.Assert(r, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrAppAlreadyExists)
}

func (s *S) TestInsertAppBackward(c *gocheck.C) {
	app := App{Name: "conviction", Platform: "evergrey"}
	ctx := action.BWContext{
		Params:   []interface{}{app},
		FWResult: &app,
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name}) // sanity
	insertApp.Backward(ctx)
	n, err := s.conn.Apps().Find(bson.M{"name": app.Name}).Count()
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 0)
}

func (s *S) TestInsertAppMinimumParams(c *gocheck.C) {
	c.Assert(insertApp.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateIAMUserForward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	app := App{Name: "trapped"}
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &app}
	result, err := createIAMUserAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(app.Name)
	u, ok := result.(*iam.User)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(u.Name, gocheck.Equals, app.Name)
}

func (s *S) TestCreateIAMUserBackward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	app := App{Name: "escape"}
	user, err := createIAMUser(app.Name)
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(user.Name)
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: user}
	createIAMUserAction.Backward(ctx)
	_, err = iamClient.GetUser(user.Name)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateIAMUserMinParams(c *gocheck.C) {
	c.Assert(createIAMUserAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateIAMAccessKeyForward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("puppets", "/")
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	ctx := action.FWContext{Params: []interface{}{nil}, Previous: &resp.User}
	result, err := createIAMAccessKeyAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	ak, ok := result.(*iam.AccessKey)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(ak.UserName, gocheck.Equals, resp.User.Name)
	c.Assert(ak.Id, gocheck.Not(gocheck.Equals), "")
	c.Assert(ak.Secret, gocheck.Equals, "")
	defer iamClient.DeleteAccessKey(ak.Id, ak.UserName)
}

func (s *S) TestCreateIAMAccessKeyBackward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("myuser", "/")
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	kresp, err := iamClient.CreateAccessKey(resp.User.Name)
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteAccessKey(kresp.AccessKey.Id, resp.User.Name)
	ctx := action.BWContext{Params: []interface{}{nil}, FWResult: &kresp.AccessKey}
	createIAMAccessKeyAction.Backward(ctx)
	akResp, err := iamClient.AccessKeys(resp.User.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(akResp.AccessKeys, gocheck.HasLen, 0)
}

func (s *S) TestCreateIAMMinParams(c *gocheck.C) {
	c.Assert(createIAMAccessKeyAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateBucketForward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{S3Endpoint: s.t.S3Server.URL()}
	s3Client := s3.New(auth, region)
	app := App{Name: "leper"}
	ctx := action.FWContext{
		Params:   []interface{}{&app},
		Previous: &iam.AccessKey{Id: "access", Secret: "s3cr3t"},
	}
	result, err := createBucketAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	env, ok := result.(*s3Env)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(env.AccessKey, gocheck.Equals, "access")
	c.Assert(env.SecretKey, gocheck.Equals, "s3cr3t")
	c.Assert(env.endpoint, gocheck.Equals, s.t.S3Server.URL())
	c.Assert(env.locationConstraint, gocheck.Equals, true)
	defer s3Client.Bucket(env.bucket).DelBucket()
	_, err = s3Client.Bucket(env.bucket).List("", "/", "", 100)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateBucketBackward(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name,
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: &env}
	createBucketAction.Backward(ctx)
	_, err = s3Client.Bucket(app.Name).List("", "/", "", 100)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateBucketMinParams(c *gocheck.C) {
	c.Assert(createBucketAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateUserPolicyForward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("blackened", "/")
	c.Assert(err, gocheck.IsNil)
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
	c.Assert(err, gocheck.IsNil)
	e, ok := result.(*s3Env)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e, gocheck.Equals, &env)
	_, err = iamClient.GetUserPolicy(resp.User.Name, "app-blackened-bucket")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateUserPolicyBackward(c *gocheck.C) {
	auth := aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"}
	region := aws.Region{IAMEndpoint: s.t.IamServer.URL()}
	iamClient := iam.New(auth, region)
	resp, err := iamClient.CreateUser("blackened", "/")
	c.Assert(err, gocheck.IsNil)
	defer iamClient.DeleteUser(resp.User.Name)
	app := App{Name: resp.User.Name}
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name,
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	_, err = iamClient.PutUserPolicy(resp.User.Name, "app-blackened-bucket", "null")
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: &env}
	createUserPolicyAction.Backward(ctx)
	_, err = iamClient.GetUserPolicy(resp.User.Name, "app-blackened-bucket")
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateUsePolicyMinParams(c *gocheck.C) {
	c.Assert(createUserPolicyAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestExportEnvironmentsForward(c *gocheck.C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "mist", Platform: "opeth"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	env := s3Env{
		Auth:               aws.Auth{AccessKey: "access", SecretKey: "s3cr3t"},
		bucket:             app.Name + "-bucket",
		endpoint:           s.t.S3Server.URL(),
		locationConstraint: true,
	}
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &env}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, &env)
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
	appEnv := app.InstanceEnv(s3InstanceName)
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Value, gocheck.Equals, env.endpoint)
	c.Assert(appEnv["TSURU_S3_ENDPOINT"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Value, gocheck.Equals, "true")
	c.Assert(appEnv["TSURU_S3_LOCATIONCONSTRAINT"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_S3_ACCESS_KEY_ID"].Value, gocheck.Equals, env.AccessKey)
	c.Assert(appEnv["TSURU_S3_ACCESS_KEY_ID"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_S3_SECRET_KEY"].Value, gocheck.Equals, env.SecretKey)
	c.Assert(appEnv["TSURU_S3_SECRET_KEY"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Value, gocheck.Equals, env.bucket)
	c.Assert(appEnv["TSURU_S3_BUCKET"].Public, gocheck.Equals, false)
	appEnv = app.InstanceEnv("")
	c.Assert(appEnv["TSURU_APPNAME"].Value, gocheck.Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_APP_TOKEN"].Value, gocheck.Not(gocheck.Equals), "")
	c.Assert(appEnv["TSURU_APP_TOKEN"].Public, gocheck.Equals, false)
	t, err := auth.GetToken("bearer " + appEnv["TSURU_APP_TOKEN"].Value)
	c.Assert(err, gocheck.IsNil)
	c.Assert(t.AppName, gocheck.Equals, app.Name)
	message, err := aqueue().Get(2e9)
	c.Assert(err, gocheck.IsNil)
	defer message.Delete()
	c.Assert(message.Action, gocheck.Equals, regenerateApprc)
	c.Assert(message.Args, gocheck.DeepEquals, []string{app.Name})
}

func (s *S) TestExportEnvironmentsForwardWithoutS3Env(c *gocheck.C) {
	expectedHost := "localhost"
	config.Set("host", expectedHost)
	app := App{Name: "mist", Platform: "opeth"}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app}, Previous: &app}
	result, err := exportEnvironmentsAction.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result, gocheck.Equals, &app)
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
	appEnv := app.InstanceEnv(s3InstanceName)
	c.Assert(appEnv, gocheck.DeepEquals, map[string]bind.EnvVar{})
	appEnv = app.InstanceEnv("")
	c.Assert(appEnv["TSURU_APPNAME"].Value, gocheck.Equals, app.Name)
	c.Assert(appEnv["TSURU_APPNAME"].Public, gocheck.Equals, false)
	c.Assert(appEnv["TSURU_HOST"].Value, gocheck.Equals, expectedHost)
	c.Assert(appEnv["TSURU_HOST"].Public, gocheck.Equals, false)
}

func (s *S) TestExportEnvironmentsBackward(c *gocheck.C) {
	envNames := []string{
		"TSURU_S3_ACCESS_KEY_ID", "TSURU_S3_SECRET_KEY",
		"TSURU_APPNAME", "TSURU_HOST", "TSURU_S3_ENDPOINT",
		"TSURU_S3_LOCATIONCONSTRAINT", "TSURU_S3_BUCKET",
		"TSURU_APP_TOKEN",
	}
	app := App{Name: "moon", Platform: "opeth", Env: make(map[string]bind.EnvVar)}
	for _, name := range envNames {
		envVar := bind.EnvVar{Name: name, Value: name, Public: false}
		if strings.HasPrefix(name, "TSURU_S3_") {
			envVar.InstanceName = s3InstanceName
		}
		app.Env[name] = envVar
	}
	token, err := auth.CreateApplicationToken(app.Name)
	c.Assert(err, gocheck.IsNil)
	app.Env["TSURU_APP_TOKEN"] = bind.EnvVar{Name: "TSURU_APP_NAME", Value: token.Token}
	err = s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.BWContext{Params: []interface{}{&app}}
	exportEnvironmentsAction.Backward(ctx)
	copy := app
	err = copy.Get()
	c.Assert(err, gocheck.IsNil)
	for _, name := range envNames {
		if _, ok := copy.Env[name]; ok {
			c.Errorf("Variable %q should be unexported, but it's still exported.", name)
		}
	}
	_, err = auth.GetToken("bearer " + token.Token)
	c.Assert(err, gocheck.Equals, auth.ErrInvalidToken)
}

func (s *S) TestExportEnvironmentsMinParams(c *gocheck.C) {
	c.Assert(exportEnvironmentsAction.MinParams, gocheck.Equals, 1)
}

func (s *S) TestCreateRepositoryForward(c *gocheck.C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/repository")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestCreateRepositoryForwardAppPointer(c *gocheck.C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp", Teams: []string{s.team.Name}}
	ctx := action.FWContext{Params: []interface{}{&app}}
	result, err := createRepository.Forward(ctx)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.url[0], gocheck.Equals, "/repository")
	c.Assert(h.method[0], gocheck.Equals, "POST")
	expected := fmt.Sprintf(`{"name":"someapp","users":["%s"],"ispublic":false}`, s.user.Email)
	c.Assert(string(h.body[0]), gocheck.Equals, expected)
}

func (s *S) TestCreateRepositoryForwardInvalidType(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{"something"}}
	_, err := createRepository.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be App or *App.")
}

func (s *S) TestCreateRepositoryBackward(c *gocheck.C) {
	h := testHandler{}
	ts := s.t.StartGandalfTestServer(&h)
	defer ts.Close()
	app := App{Name: "someapp"}
	ctx := action.BWContext{FWResult: &app, Params: []interface{}{app}}
	createRepository.Backward(ctx)
	c.Assert(h.url[0], gocheck.Equals, "/repository/someapp")
	c.Assert(h.method[0], gocheck.Equals, "DELETE")
	c.Assert(string(h.body[0]), gocheck.Equals, "null")
}

func (s *S) TestCreateRepositoryMinParams(c *gocheck.C) {
	c.Assert(createRepository.MinParams, gocheck.Equals, 1)
}

func (s *S) TestProvisionAppForward(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
		Units:    []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, gocheck.IsNil)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, true)
}

func (s *S) TestProvisionAppForwardAppPointer(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
		Units:    []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	ctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(ctx)
	defer s.provisioner.Destroy(&app)
	c.Assert(err, gocheck.IsNil)
	a, ok := result.(*App)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(a.Name, gocheck.Equals, app.Name)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, true)
}

func (s *S) TestProvisionAppForwardInvalidApp(c *gocheck.C) {
	ctx := action.FWContext{Params: []interface{}{"something", 1}}
	_, err := provisionApp.Forward(ctx)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestProvisionAppBackward(c *gocheck.C) {
	app := App{
		Name:     "earthshine",
		Platform: "django",
		Units:    []Unit{{Machine: 3}},
	}
	err := s.conn.Apps().Insert(app)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	fwctx := action.FWContext{Params: []interface{}{&app, 4}}
	result, err := provisionApp.Forward(fwctx)
	c.Assert(err, gocheck.IsNil)
	bwctx := action.BWContext{Params: []interface{}{&app, 4}, FWResult: result}
	provisionApp.Backward(bwctx)
	c.Assert(s.provisioner.Provisioned(&app), gocheck.Equals, false)
}

func (s *S) TestProvisionAppMinParams(c *gocheck.C) {
	c.Assert(provisionApp.MinParams, gocheck.Equals, 1)
}

func (s *S) TestReserveUserAppForward(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	err := quota.Create(user.Email, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(user.Email)
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, &user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = quota.Reserve(user.Email, "another-app")
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = quota.Release(user.Email, app.Name)
	c.Assert(err, gocheck.IsNil)
	err = quota.Reserve(user.Email, "another-app")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardNonPointer(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	err := quota.Create(user.Email, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(user.Email)
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{&app, user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = quota.Reserve(user.Email, "another-app")
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = quota.Release(user.Email, app.Name)
	c.Assert(err, gocheck.IsNil)
	err = quota.Reserve(user.Email, "another-app")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardAppNotPointer(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	err := quota.Create(user.Email, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(user.Email)
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"user": user.Email, "app": app.Name}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{app, user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
	err = quota.Reserve(user.Email, "another-app")
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
	err = quota.Release(user.Email, app.Name)
	c.Assert(err, gocheck.IsNil)
	err = quota.Reserve(user.Email, "another-app")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppForwardInvalidApp(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{"something", user}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be App or *App.")
}

func (s *S) TestReserveUserAppForwardInvalidUser(c *gocheck.C) {
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{app, "something"}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Third parameter must be auth.User or *auth.User.")
}

func (s *S) TestReserveUserAppForwardQuotaExceeded(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	err := quota.Create(user.Email, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(user.Email)
	err = quota.Reserve(user.Email, "anything")
	c.Assert(err, gocheck.IsNil)
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{app, user}})
	c.Assert(previous, gocheck.IsNil)
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestReserveUserAppForwardQuotaNotFound(c *gocheck.C) {
	user := auth.User{Email: "south@yes.com"}
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	expected := map[string]string{"app": app.Name, "user": user.Email}
	previous, err := reserveUserApp.Forward(action.FWContext{Params: []interface{}{app, user}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.DeepEquals, expected)
}

func (s *S) TestReserveUserAppBackward(c *gocheck.C) {
	user := auth.User{Email: "clap@yes.com"}
	err := quota.Create(user.Email, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(user.Email)
	app := App{
		Name:     "clap",
		Platform: "django",
	}
	err = quota.Reserve(user.Email, app.Name)
	c.Assert(err, gocheck.IsNil)
	ctx := action.BWContext{
		FWResult: map[string]string{
			"app":  app.Name,
			"user": user.Email,
		},
	}
	reserveUserApp.Backward(ctx)
	err = quota.Reserve(user.Email, app.Name)
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestReserveUserAppMinParams(c *gocheck.C) {
	c.Assert(reserveUserApp.MinParams, gocheck.Equals, 2)
}

func (s *S) TestCreateAppQuotaForward(c *gocheck.C) {
	config.Set("quota:units-per-app", 2)
	defer config.Unset("quota:units-per-app")
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	defer quota.Delete(app.Name)
	previous, err := createAppQuota.Forward(action.FWContext{Params: []interface{}{app}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.Equals, app.Name)
	items, available, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(items, gocheck.DeepEquals, []string{"visions-0"})
	c.Assert(available, gocheck.Equals, uint(1))
	err = quota.Reserve(app.Name, "visions-1")
	c.Assert(err, gocheck.IsNil)
	err = quota.Reserve(app.Name, "visions-2")
	_, ok := err.(*quota.QuotaExceededError)
	c.Assert(ok, gocheck.Equals, true)
}

func (s *S) TestCreateAppQuotaForwardPointer(c *gocheck.C) {
	config.Set("quota:units-per-app", 2)
	defer config.Unset("quota:units-per-app")
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	defer quota.Delete(app.Name)
	previous, err := createAppQuota.Forward(action.FWContext{Params: []interface{}{&app}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.Equals, app.Name)
	err = quota.Reserve(app.Name, "visions/0")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestCreateAppForwardWithoutSetting(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	previous, err := createAppQuota.Forward(action.FWContext{Params: []interface{}{&app}})
	c.Assert(err, gocheck.IsNil)
	c.Assert(previous, gocheck.Equals, app.Name)
	err = quota.Reserve(app.Name, "visions/0")
	c.Assert(err, gocheck.Equals, quota.ErrQuotaNotFound)
}

func (s *S) TestCreateAppForwardZeroUnits(c *gocheck.C) {
	config.Set("quota:units-per-app", 0)
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	previous, err := createAppQuota.Forward(action.FWContext{Params: []interface{}{&app}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "app creation is disallowed")
	err = quota.Reserve(app.Name, "visions/0")
	c.Assert(err, gocheck.Equals, quota.ErrQuotaNotFound)
}

func (s *S) TestCreateAppQuotaForwardPointerInvalidApp(c *gocheck.C) {
	previous, err := createAppQuota.Forward(action.FWContext{Params: []interface{}{"something"}})
	c.Assert(previous, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestCreateAppQuotaBackward(c *gocheck.C) {
	app := App{
		Name:     "damned",
		Platform: "django",
	}
	err := quota.Create(app.Name, 1)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	createAppQuota.Backward(action.BWContext{FWResult: app.Name})
	err = quota.Reserve(app.Name, "something")
	c.Assert(err, gocheck.Equals, quota.ErrQuotaNotFound)
}

func (s *S) TestCreateAppQuotaMinParams(c *gocheck.C) {
	c.Assert(createAppQuota.MinParams, gocheck.Equals, 1)
}

func (s *S) TestReserveUnitsToAddForward(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := quota.Create(app.Name, 5)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(err, gocheck.IsNil)
	ids, ok := result.([]string)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(ids, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
	items, avail, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(avail, gocheck.Equals, uint(2))
	c.Assert(items, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
}

func (s *S) TestReserveUnitsToAddForwardUint(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := quota.Create(app.Name, 5)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, uint(3)}})
	c.Assert(err, gocheck.IsNil)
	ids, ok := result.([]string)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(ids, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
	items, avail, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(avail, gocheck.Equals, uint(2))
	c.Assert(items, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
}

func (s *S) TestReserveUnitsToAddForwardNoPointer(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	err := quota.Create(app.Name, 5)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{app, 3}})
	c.Assert(err, gocheck.IsNil)
	ids, ok := result.([]string)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(ids, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
	items, avail, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(avail, gocheck.Equals, uint(2))
	c.Assert(items, gocheck.DeepEquals, []string{"visions-0", "visions-1", "visions-2"})
}

func (s *S) TestReserveUnitsToAddForwardInvalidApp(c *gocheck.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{"something", 3}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be App or *App.")
}

func (s *S) TestReserveUnitsToAddAppNotFound(c *gocheck.C) {
	app := App{Name: "something"}
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{&app, 3}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "App not found")
}

func (s *S) TestReserveUnitsToAddForwardInvalidNumber(c *gocheck.C) {
	result, err := reserveUnitsToAdd.Forward(action.FWContext{Params: []interface{}{App{}, "what"}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Second parameter must be int or uint.")
}

func (s *S) TestReserveUnitsToAddBackward(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	err := quota.Create(app.Name, 5)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	ids := []string{"visions-0", "visions-1", "visions-2", "visions-3"}
	err = quota.Reserve(app.Name, ids...)
	c.Assert(err, gocheck.IsNil)
	reserveUnitsToAdd.Backward(action.BWContext{Params: []interface{}{&app, 3}, FWResult: ids})
	items, avail, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(avail, gocheck.Equals, uint(5))
	c.Assert(items, gocheck.HasLen, 0)
}

func (s *S) TestReserveUnitsToAddBackwardNoPointer(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	err := quota.Create(app.Name, 5)
	c.Assert(err, gocheck.IsNil)
	defer quota.Delete(app.Name)
	ids := []string{"visions-0", "visions-1", "visions-2", "visions-3"}
	err = quota.Reserve(app.Name, ids...)
	c.Assert(err, gocheck.IsNil)
	reserveUnitsToAdd.Backward(action.BWContext{Params: []interface{}{app, 3}, FWResult: ids})
	items, avail, err := quota.Items(app.Name)
	c.Assert(err, gocheck.IsNil)
	c.Assert(avail, gocheck.Equals, uint(5))
	c.Assert(items, gocheck.HasLen, 0)
}

func (s *S) TestReserveUnitsMinParams(c *gocheck.C) {
	c.Assert(reserveUnitsToAdd.MinParams, gocheck.Equals, 2)
}

func (s *S) TestProvisionAddUnits(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	ctx := action.FWContext{
		Previous: []string{"visions-0", "visions-1", "visions-2"},
		Params:   []interface{}{&app},
	}
	fwresult, err := provisionAddUnits.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	result, ok := fwresult.(*addUnitsActionResult)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(result.ids, gocheck.DeepEquals, ctx.Previous)
	c.Assert(result.units, gocheck.HasLen, 3)
	c.Assert(result.units, gocheck.DeepEquals, s.provisioner.GetUnits(&app)[1:])
}

func (s *S) TestProvisionAddUnitsNoPointer(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	ctx := action.FWContext{
		Previous: []string{"visions-0", "visions-1", "visions-2"},
		Params:   []interface{}{app},
	}
	fwresult, err := provisionAddUnits.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	result, ok := fwresult.(*addUnitsActionResult)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(result.ids, gocheck.DeepEquals, ctx.Previous)
	c.Assert(result.units, gocheck.HasLen, 3)
	c.Assert(result.units, gocheck.DeepEquals, s.provisioner.GetUnits(&app)[1:])
}

func (s *S) TestProvisionAddUnitsProvisionFailure(c *gocheck.C) {
	s.provisioner.PrepareFailure("AddUnits", errors.New("Failed to add units"))
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	ctx := action.FWContext{
		Previous: []string{"visions-0", "visions-1", "visions-2"},
		Params:   []interface{}{app},
	}
	result, err := provisionAddUnits.Forward(ctx)
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "Failed to add units")
}

func (s *S) TestProvisionAddUnitsInvalidApp(c *gocheck.C) {
	result, err := provisionAddUnits.Forward(action.FWContext{Params: []interface{}{"something"}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "First parameter must be App or *App.")
}

func (s *S) TestProvisionAddUnitsBackward(c *gocheck.C) {
	app := App{
		Name:     "fiction",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	units, err := s.provisioner.AddUnits(&app, 3)
	c.Assert(err, gocheck.IsNil)
	result := addUnitsActionResult{ids: []string{"unit-0", "unit-1"}, units: units}
	ctx := action.BWContext{Params: []interface{}{&app}, FWResult: &result}
	provisionAddUnits.Backward(ctx)
	c.Assert(s.provisioner.GetUnits(&app), gocheck.HasLen, 1)
}

func (s *S) TestProvisionAddUnitsBackwardNoPointer(c *gocheck.C) {
	app := App{
		Name:     "fiction",
		Platform: "django",
	}
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	units, err := s.provisioner.AddUnits(&app, 3)
	c.Assert(err, gocheck.IsNil)
	result := addUnitsActionResult{ids: []string{"unit-0", "unit-1"}, units: units}
	ctx := action.BWContext{Params: []interface{}{app}, FWResult: &result}
	provisionAddUnits.Backward(ctx)
	c.Assert(s.provisioner.GetUnits(&app), gocheck.HasLen, 1)
}

func (s *S) TestProvisionAddUnitsMinParams(c *gocheck.C) {
	c.Assert(provisionAddUnits.MinParams, gocheck.Equals, 1)
}

func (s *S) TestSaveNewUnitsInDatabaseForward(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	units, err := s.provisioner.AddUnits(&app, 3)
	c.Assert(err, gocheck.IsNil)
	ids := []string{"unit-0", "unit-1", "unit-2"}
	result := addUnitsActionResult{ids: ids, units: units}
	ctx := action.FWContext{Previous: &result, Params: []interface{}{&app}}
	fwresult, err := saveNewUnitsInDatabase.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fwresult, gocheck.IsNil)
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units, gocheck.HasLen, 3)
	var expectedMessages MessageList
	for i, unit := range app.Units {
		c.Assert(unit.Name, gocheck.Equals, units[i].Name)
		c.Assert(unit.QuotaItem, gocheck.Equals, ids[i])
		messages := []queue.Message{
			{Action: RegenerateApprcAndStart, Args: []string{app.Name, unit.Name}},
			{Action: bindService, Args: []string{app.Name, unit.Name}},
		}
		expectedMessages = append(expectedMessages, messages...)
	}
	gotMessages := make(MessageList, expectedMessages.Len())
	for i := range expectedMessages {
		message, err := aqueue().Get(1e6)
		c.Assert(err, gocheck.IsNil)
		defer message.Delete()
		gotMessages[i] = queue.Message{
			Action: message.Action,
			Args:   message.Args,
		}
	}
	sort.Sort(expectedMessages)
	sort.Sort(gotMessages)
	c.Assert(gotMessages, gocheck.DeepEquals, expectedMessages)
}

func (s *S) TestSaveNewUnitsInDatabaseForwardNoPointer(c *gocheck.C) {
	app := App{
		Name:     "visions",
		Platform: "django",
	}
	s.conn.Apps().Insert(app)
	defer s.conn.Apps().Remove(bson.M{"name": app.Name})
	s.provisioner.Provision(&app)
	defer s.provisioner.Destroy(&app)
	units, err := s.provisioner.AddUnits(&app, 3)
	c.Assert(err, gocheck.IsNil)
	ids := []string{"unit-0", "unit-1", "unit-2"}
	result := addUnitsActionResult{ids: ids, units: units}
	ctx := action.FWContext{Previous: &result, Params: []interface{}{app}}
	fwresult, err := saveNewUnitsInDatabase.Forward(ctx)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fwresult, gocheck.IsNil)
	err = app.Get()
	c.Assert(err, gocheck.IsNil)
	c.Assert(err, gocheck.IsNil)
	c.Assert(app.Units, gocheck.HasLen, 3)
	var expectedMessages MessageList
	for i, unit := range app.Units {
		c.Assert(unit.Name, gocheck.Equals, units[i].Name)
		c.Assert(unit.QuotaItem, gocheck.Equals, ids[i])
		messages := []queue.Message{
			{Action: RegenerateApprcAndStart, Args: []string{app.Name, unit.Name}},
			{Action: bindService, Args: []string{app.Name, unit.Name}},
		}
		expectedMessages = append(expectedMessages, messages...)
	}
	gotMessages := make(MessageList, expectedMessages.Len())
	for i := range expectedMessages {
		message, err := aqueue().Get(1e6)
		c.Assert(err, gocheck.IsNil)
		defer message.Delete()
		gotMessages[i] = queue.Message{
			Action: message.Action,
			Args:   message.Args,
		}
	}
	sort.Sort(expectedMessages)
	sort.Sort(gotMessages)
	c.Assert(gotMessages, gocheck.DeepEquals, expectedMessages)
}

func (s *S) TestSaveNewUnitsInDatabaseForwardInvalidApp(c *gocheck.C) {
	result, err := saveNewUnitsInDatabase.Forward(action.FWContext{Params: []interface{}{"something"}})
	c.Assert(result, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSaveNewUnitsInDatabaseAppNotFound(c *gocheck.C) {
	app := App{Name: "something"}
	fwresult := addUnitsActionResult{}
	ctx := action.FWContext{Previous: &fwresult, Params: []interface{}{app}}
	result, err := saveNewUnitsInDatabase.Forward(ctx)
	c.Assert(result, gocheck.IsNil)
	c.Assert(err.Error(), gocheck.Equals, "App not found")
}

func (s *S) TestSaveNewUnitsInDatabaseBackward(c *gocheck.C) {
	c.Assert(saveNewUnitsInDatabase.Backward, gocheck.IsNil)
}

func (s *S) TestSaveNewUnitsMinParams(c *gocheck.C) {
	c.Assert(saveNewUnitsInDatabase.MinParams, gocheck.Equals, 1)
}
