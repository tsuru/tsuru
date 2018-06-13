package mongodb

import (
	"github.com/tsuru/tsuru/storage/storagetest"
	check "gopkg.in/check.v1"
)

var _ = check.Suite(&storagetest.ServiceBrokerSuite{
	ServiceBrokerStorage: &serviceBrokerStorage{},
	SuiteHooks:           &mongodbBaseTest{},
})
