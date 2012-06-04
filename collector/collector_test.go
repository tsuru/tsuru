package collector

import (
	"github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/db"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
	tmpdir  string
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.tmpdir, err = commandmocker.Add("juju", "")
	c.Assert(err, IsNil)
	db.Session, err = db.Open("127.0.0.1:27017", "tsuru_collector_test")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	defer commandmocker.Remove(s.tmpdir)
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	err := db.Session.Apps().RemoveAll(nil)
	c.Assert(err, IsNil)
}

func (s *S) TestCollectorUpdate(c *C) {
	a := app.App{}
	a.Name = "umaappqq"
	a.State = "STOPPED"
	err := a.Create()
	c.Assert(err, IsNil)

	var collector Collector

	out := &output{
		Services: map[string]Service{
			"umaappqq": Service{
				Units: map[string]Unit{
					"umaappqq/0": Unit{
						State:   "started",
						Machine: 1,
					},
				},
			},
		},
		Machines: map[int]interface{}{
			0: map[interface{}]interface{}{
				"dns-name":       "192.168.0.10",
				"instance-id":    "i-00000zz6",
				"instance-state": "running",
				"agent-state":    "running",
			},
			1: map[interface{}]interface{}{
				"dns-name":       "192.168.0.11",
				"instance-id":    "i-00000zz7",
				"instance-state": "running",
				"agent-state":    "running",
			},
		},
	}

	collector.Update(out)

	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, DeepEquals, "STARTED")
	c.Assert(a.Ip, DeepEquals, "192.168.0.11")
	c.Assert(a.Machine, DeepEquals, 1)

	a.Destroy()
}

func (s *S) TestCollectorParser(c *C) {
	var collector Collector

	file, _ := os.Open(filepath.Join("testdata", "output.yaml"))
	jujuOutput, _ := ioutil.ReadAll(file)
	file.Close()

	expected := &output{
		Services: map[string]Service{
			"umaappqq": Service{
				Units: map[string]Unit{
					"umaappqq/0": Unit{
						State:   "started",
						Machine: 1,
					},
				},
			},
		},
		Machines: map[int]interface{}{
			0: map[interface{}]interface{}{
				"dns-name":       "192.168.0.10",
				"instance-id":    "i-00000zz6",
				"instance-state": "running",
				"agent-state":    "running",
			},
			1: map[interface{}]interface{}{
				"dns-name":       "192.168.0.11",
				"instance-id":    "i-00000zz7",
				"instance-state": "running",
				"agent-state":    "running",
			},
		},
	}

	c.Assert(collector.Parse(jujuOutput), DeepEquals, expected)
}
