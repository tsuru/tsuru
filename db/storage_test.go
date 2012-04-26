package db

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{
	storage *Storage
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	s.storage, _ = Open("127.0.0.1:27017", "tsuru_storage_test")
}

func (s *S) TearDownSuite(c *C) {
	defer s.storage.Close()
	s.storage.session.DB("tsuru").DropDatabase()
}

func (s *S) TestShouldProvideMethodToOpenAConnection(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	c.Assert(storage.session.Ping(), IsNil)
	storage.Close()
}

func (s *S) TestMethodCloseSholdCloseTheConnectionWithMongoDB(c *C) {
	defer func() {
		if r := recover(); r == nil {
			c.Errorf("Should close the connection, but did not!")
		}
	}()
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	storage.Close()
	storage.session.Ping()
}

func (s *S) TestShouldProvidePrivateMethodToGetACollection(c *C) {
	collection := s.storage.getCollection("users")
	c.Assert(collection.FullName, Equals, s.storage.dbname + ".users")
}

func (s *S) TestShouldCacheCollection(c *C) {
	collection := s.storage.getCollection("users")
	c.Assert(collection, DeepEquals, s.storage.collections["users"])
}

func (s *S) TestMethodUsersShouldReturnUsersCollection(c *C) {
	users := s.storage.Users()
	usersc := s.storage.getCollection("users")
	c.Assert(users, DeepEquals, usersc)
}

func (s *S) TestMethodUserShouldReturnUsersCollectionWithUniqueIndexForEmail(c *C) {
	users := s.storage.Users()
	indexes, err := users.Indexes()
	c.Assert(err, IsNil)
	found := false
	for _, index := range indexes {
		for _, key := range index.Key {
			if key == "email" {
				c.Assert(index.Unique, Equals, true)
				found = true
				break
			}
		}

		if found {
			break
		}
	}

	if !found {
		c.Errorf("Users should declare a unique index for email")
	}
}

func (s *S) TestMethodAppsShouldReturnAppsCollection(c *C) {
	apps := s.storage.Apps()
	appsc := s.storage.getCollection("apps")
	c.Assert(apps, DeepEquals, appsc)
}

func (s *S) TestMethodAppsShouldReturnAppsCollectionWithUniqueIndexForName(c *C) {
	storage, _ := Open("127.0.0.1:27017", "tsuru_storage_test")
	defer storage.Close()
	apps := storage.Apps()
	indexes, err := apps.Indexes()
	c.Assert(err, IsNil)
	found := false
	for _, index := range indexes {
		for _, key := range index.Key {
			if key == "name" {
				c.Assert(index.Unique, Equals, true)
				found = true
				break
			}
		}

		if found {
			break
		}
	}

	if !found {
		c.Errorf("Apps should declare a unique index for name")
	}
}

func (s *S) TestMethodServicesShouldReturnServicesCollection(c *C) {
	services := s.storage.Services()
	servicesc := s.storage.getCollection("services")
	c.Assert(services, DeepEquals, servicesc)
}

func (s *S) TestMethodServiceAppsShouldReturnServiceAppsCollection(c *C) {
	serviceApps := s.storage.ServiceApps()
	serviceAppsc := s.storage.getCollection("service_apps")
	c.Assert(serviceApps, DeepEquals, serviceAppsc)
}

func (s *S) TestMethodServiceTypesReturnServiceTypesCollection(c *C) {
	serviceTypes := s.storage.ServiceTypes()
	serviceTypesc := s.storage.getCollection("service_types")
	c.Assert(serviceTypes, DeepEquals, serviceTypesc)
}

func (s *S) TestMethodUnitsShouldReturnUnitsCollection(c *C) {
	units := s.storage.Units()
	unitsc := s.storage.getCollection("units")
	c.Assert(units, DeepEquals, unitsc)
}
