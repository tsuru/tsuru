package auth

import (
	"bytes"
	"github.com/globocom/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/repository"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
)

type hasKeyChecker struct{}

func (c *hasKeyChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasKey", Params: []string{"user", "key"}}
}

func (c *hasKeyChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should provide two parameters"
	}
	user, ok := params[0].(*User)
	if !ok {
		return false, "first parameter should be a user pointer"
	}
	content, ok := params[1].(string)
	if !ok {
		return false, "second parameter should be a string"
	}
	key := Key{Content: content}
	return user.hasKey(key), ""
}

var HasKey Checker = &hasKeyChecker{}

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session     *mgo.Session
	user        *User
	team        *Team
	token       *Token
	gitRoot     string
	gitosisBare string
	gitosisRepo string
}

var _ = Suite(&S{})

var panicIfErr = func(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *S) setupGitosis() {
	data, err := ioutil.ReadFile("../../etc/tsuru.conf")
	panicIfErr(err)
	data = bytes.Replace(data, []byte("/tmp/git"), []byte("/tmp/gitosis"), -1)
	err = config.ReadConfigBytes(data)
	panicIfErr(err)
	s.gitRoot, err = config.GetString("git:root")
	panicIfErr(err)
	s.gitosisBare, err = config.GetString("git:gitosis-bare")
	panicIfErr(err)
	s.gitosisRepo, err = config.GetString("git:gitosis-repo")
	err = os.RemoveAll(s.gitRoot)
	panicIfErr(err)
	err = os.MkdirAll(s.gitRoot, 0777)
	panicIfErr(err)
	err = exec.Command("git", "init", "--bare", s.gitosisBare).Run()
	panicIfErr(err)
	err = exec.Command("git", "clone", s.gitosisBare, s.gitosisRepo).Run()
	panicIfErr(err)
}

func (s *S) tearDownGitosis() {
	err := os.RemoveAll(s.gitRoot)
	panicIfErr(err)
}

func (s *S) commit(msg string) {
	ch := repository.Change{
		Kind:     repository.Commit,
		Args:     map[string]string{"message": msg},
		Response: make(chan string),
	}
	repository.Ag.Process(ch)
	<-ch.Response
}

func (s *S) createGitosisConf() {
	p := path.Join(s.gitosisRepo, "gitosis.conf")
	f, err := os.Create(p)
	panicIfErr(err)
	defer f.Close()
	s.commit("Added gitosis.conf")
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

func (s *S) deleteGitosisConf() {
	err := os.Remove(path.Join(s.gitosisRepo, "gitosis.conf"))
	panicIfErr(err)
	s.commit("Removing gitosis.conf")
}

func (s *S) SetUpSuite(c *C) {
	s.setupGitosis()
	repository.RunAgent()
	db.Session, _ = db.Open("localhost:27017", "tsuru_user_test")
	s.user = &User{Email: "timeredbull@globo.com", Password: "123"}
	s.user.Create()
	s.token, _ = s.user.CreateToken()
	err := createTeam("cobrateam", s.user)
	panicIfErr(err)
	s.team = new(Team)
	err = db.Session.Teams().Find(bson.M{"_id": "cobrateam"}).One(s.team)
	panicIfErr(err)
}

func (s *S) TearDownSuite(c *C) {
	defer s.tearDownGitosis()
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) SetUpTest(c *C) {
	s.createGitosisConf()
}

func (s *S) TearDownTest(c *C) {
	defer s.deleteGitosisConf()
	_, err := db.Session.Users().RemoveAll(bson.M{"email": bson.M{"$ne": s.user.Email}})
	panicIfErr(err)
	_, err = db.Session.Teams().RemoveAll(bson.M{"_id": bson.M{"$ne": s.team.Name}})
	panicIfErr(err)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}
