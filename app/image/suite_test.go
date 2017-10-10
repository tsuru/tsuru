package image

import (
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	storage *db.Storage
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "app_image_tests")
	config.Set("docker:collection", "docker")
	config.Set("docker:repository-namespace", "tsuru")
	var err error
	s.storage, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownTest(c *check.C) {
	s.storage.Apps().Database.DropDatabase()
}

func (s *S) TearDownSuite(c *check.C) {
	s.storage.Close()
}
