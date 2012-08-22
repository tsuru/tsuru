package app

import (
	"bytes"
	"fmt"
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	fsTesting "github.com/timeredbull/tsuru/fs/testing"
	"github.com/timeredbull/tsuru/repository"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"net/http/httptest"
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
	tmpdir      string
	ts          *httptest.Server
	rfs         *fsTesting.RecordingFs
}

var _ = Suite(&S{})

type greaterChecker struct{}

func (c *greaterChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "Greater", Params: []string{"expected", "obtained"}}
}

func (c *greaterChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should pass two values to compare"
	}
	n1, ok := params[0].(int)
	if !ok {
		return false, "first parameter should be int"
	}
	n2, ok := params[1].(int)
	if !ok {
		return false, "second parameter should be int"
	}
	if n1 > n2 {
		return true, ""
	}
	err := fmt.Sprintf("%s is not greater than %s", params[0], params[1])
	return false, err
}

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

var IsInGitosis, NotInGitosis, Greater Checker = &isInGitosisChecker{}, Not(IsInGitosis), &greaterChecker{}

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
		Kind:     repository.Commit,
		Args:     map[string]string{"message": msg},
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
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_app_test")
	c.Assert(err, IsNil)
	s.user = &auth.User{Email: "whydidifall@thewho.com", Password: "123"}
	s.user.Create()
	s.team = auth.Team{Name: "tsuruteam", Users: []string{s.user.Email}}
	db.Session.Teams().Insert(s.team)
	s.setupGitosis(c)
	repository.RunAgent()
	s.rfs = &fsTesting.RecordingFs{}
	file, err := s.rfs.Open("/dev/urandom")
	c.Assert(err, IsNil)
	file.Write([]byte{16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31})
	fsystem = s.rfs
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer s.tearDownGitosis(c)
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
	fsystem = nil
}

func (s *S) SetUpTest(c *C) {
	s.createGitosisConf(c)
	s.ts = s.mockServer("", "")
}

func (s *S) TearDownTest(c *C) {
	defer s.deleteGitosisConf(c)
	var apps []App
	err := db.Session.Apps().Find(nil).All(&apps)
	c.Assert(err, IsNil)
	_, err = db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
	for _, app := range apps {
		app.Destroy()
	}
	Client.Token = ""
	s.ts.Close()
}

func (s *S) getTestData(p ...string) io.ReadCloser {
	p = append([]string{}, ".", "testdata")
	fp := path.Join(p...)
	f, _ := os.OpenFile(fp, os.O_RDONLY, 0)
	return f
}
