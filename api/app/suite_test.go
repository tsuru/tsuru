package app

import (
	"bytes"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/repository"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session     *mgo.Session
	team        auth.Team
	user        *auth.User
	gitRoot     string
	gitosisBare string
	gitosisRepo string
}

var _ = Suite(&S{})

type isInGitosisChecker struct{}

func (c *isInGitosisChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "IsInGitosis", Params: []string{"str"}}
}

func (c *isInGitosisChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 1 {
		return false, "you should provide one string parameter"
	}
	str, ok := params[0].(string)
	if !ok {
		return false, "the parameter should be a string"
	}
	gitosisRepo, err := config.GetString("git:gitosis-repo")
	if err != nil {
		return false, "failed to get config"
	}
	path := path.Join(gitosisRepo, "gitosis.conf")
	f, err := os.Open(path)
	if err != nil {
		return false, err.Error()
	}
	defer f.Close()
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return false, err.Error()
	}
	return strings.Contains(string(content), str), ""
}

var IsInGitosis, NotInGitosis Checker = &isInGitosisChecker{}, Not(IsInGitosis)

func (s *S) setupGitosis(c *C) {
	data, err := ioutil.ReadFile("../../etc/tsuru.conf")
	c.Assert(err, IsNil)
	data = bytes.Replace(data, []byte("/tmp/git"), []byte("/tmp/gitosis_app"), -1)
	err = config.ReadConfigBytes(data)
	c.Assert(err, IsNil)
	s.gitRoot, err = config.GetString("git:root")
	c.Assert(err, IsNil)
	s.gitosisBare, err = config.GetString("git:gitosis-bare")
	c.Assert(err, IsNil)
	s.gitosisRepo, err = config.GetString("git:gitosis-repo")
	err = os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
	err = os.MkdirAll(s.gitRoot, 0777)
	c.Assert(err, IsNil)
	err = exec.Command("git", "init", "--bare", s.gitosisBare).Run()
	c.Assert(err, IsNil)
	err = exec.Command("git", "clone", s.gitosisBare, s.gitosisRepo).Run()
	c.Assert(err, IsNil)
}

func (s *S) tearDownGitosis(c *C) {
	err := os.RemoveAll(s.gitRoot)
	c.Assert(err, IsNil)
}

func (s *S) commit(c *C, msg string) {
	ch := repository.Change{
		Kind: repository.Commit,
		Args: map[string]string{"message": msg},
		Response: make(chan string),
	}
	repository.Ag.Process(ch)
	<-ch.Response
}

func (s *S) createGitosisConf(c *C) {
	p := path.Join(s.gitosisRepo, "gitosis.conf")
	f, err := os.Create(p)
	c.Assert(err, IsNil)
	defer f.Close()
	s.commit(c, "Added gitosis.conf")
}

func (s *S) addGroup() {
	ch := repository.Change{
		Kind:     repository.AddGroup,
		Args:     map[string]string{"group": s.team.Name},
		Response: make(chan string),
	}
	repository.Ag.Process(ch)
	<-ch.Response
}

func (s *S) deleteGitosisConf(c *C) {
	err := os.Remove(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	s.commit(c, "Removing gitosis.conf")
}

func (s *S) SetUpSuite(c *C) {
	var err error
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_app_test")
	c.Assert(err, IsNil)
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	s.user.Create()
	s.team = auth.Team{Name: "tsuruteam", Users: []*auth.User{s.user}}
	db.Session.Teams().Insert(s.team)
	s.setupGitosis(c)
	repository.RunAgent()
}

func (s *S) TearDownSuite(c *C) {
	defer s.tearDownGitosis(c)
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) SetUpTest(c *C) {
	s.createGitosisConf(c)
}

func (s *S) TearDownTest(c *C) {
	defer s.deleteGitosisConf(c)
	var apps []App
	err := db.Session.Apps().Find(nil).All(&apps)
	c.Assert(err, IsNil)
	for _, app := range apps {
		err = app.Destroy()
		c.Assert(err, IsNil)
	}
}
