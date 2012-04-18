package collector

import (
	"github.com/timeredbull/tsuru/api/app"
	. "github.com/timeredbull/tsuru/database"
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
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.session, err = mgo.Dial("localhost:27017")
	c.Assert(err, IsNil)
	Mdb = s.session.DB("tsuru_collector_test")
}

func (s *S) TearDownSuite(c *C) {
	err := Mdb.DropDatabase()
	c.Assert(err, IsNil)
	s.session.Close()
}

func (s *S) TearDownTest(c *C) {
	err := Mdb.C("apps").DropCollection()
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
				"state":          "running",
			},
			1: map[interface{}]interface{}{
				"dns-name":       "192.168.0.11",
				"instance-id":    "i-00000zz7",
				"instance-state": "running",
				"state":          "running",
			},
		},
	}

	collector.Update(out)

	err = a.Get()
	c.Assert(err, IsNil)
	c.Assert(a.State, DeepEquals, "STARTED")
	c.Assert(a.Ip, DeepEquals, "192.168.0.11")
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
				"state":          "running",
			},
			1: map[interface{}]interface{}{
				"dns-name":       "192.168.0.11",
				"instance-id":    "i-00000zz7",
				"instance-state": "running",
				"state":          "running",
			},
		},
	}

	c.Assert(collector.Parse(jujuOutput), DeepEquals, expected)
}
