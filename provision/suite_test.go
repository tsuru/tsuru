package provision

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	servicemock "github.com/tsuru/tsuru/servicemanager/mock"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	check "gopkg.in/check.v1"
)

type S struct {
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_tests_s")
	servicemock.SetMockService(&servicemock.MockService{})
}

func (s *S) TearDownSuite(c *check.C) {
	storagev2.ClearAllCollections(nil)
}

func (s *S) SetUpTest(c *check.C) {
	err := storagev2.ClearAllCollections(nil)
	c.Assert(err, check.IsNil)
}
