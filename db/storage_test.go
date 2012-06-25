package db

import (
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"reflect"
	"testing"
)

type hasUniqueIndexChecker struct{}

func (c *hasUniqueIndexChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasUniqueField", Params: []string{"collection", "key"}}
}

func (c *hasUniqueIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*mgo.Collection)
	if !ok {
		return false, "first parameter should be a mgo collection"
	}
	key, ok := params[1].([]string)
	if !ok {
		return false, "second parameter should be the key, as used for mgo index declaration (slice of strings)"
	}
	indexes, err := collection.Indexes()
	if err != nil {
		return false, "failed to get collection indexes: " + err.Error()
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return index.Unique, ""
		}
	}
	return false, ""
}

var HasUniqueIndex Checker = &hasUniqueIndexChecker{}

func Test(t *testing.T) { TestingT(t) }

type S struct {
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
	c.Assert(collection.FullName, Equals, s.storage.dbname+".users")
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
	c.Assert(users, HasUniqueIndex, []string{"email"})
}

func (s *S) TestMethodAppsShouldReturnAppsCollection(c *C) {
	apps := s.storage.Apps()
	appsc := s.storage.getCollection("apps")
	c.Assert(apps, DeepEquals, appsc)
}

func (s *S) TestMethodAppsShouldReturnAppsCollectionWithUniqueIndexForName(c *C) {
	apps := s.storage.Apps()
	c.Assert(apps, HasUniqueIndex, []string{"name"})
}

func (s *S) TestMethodServicesShouldReturnServicesCollection(c *C) {
	services := s.storage.Services()
	servicesc := s.storage.getCollection("services")
	c.Assert(services, DeepEquals, servicesc)
}

func (s *S) TestMethodServicesShouldReturnServicesCollectionWithUniqueIndexForName(c *C) {
	services := s.storage.Services()
	c.Assert(services, HasUniqueIndex, []string{"name"})
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

func (s *S) TestMethodTeamsShouldReturnTeamsCollection(c *C) {
	teams := s.storage.Teams()
	teamsc := s.storage.getCollection("teams")
	c.Assert(teams, DeepEquals, teamsc)
}

func (s *S) TestMethodTeamsShouldReturnTeamsCollectionWithUniqueIndexForName(c *C) {
	teams := s.storage.Teams()
	c.Assert(teams, HasUniqueIndex, []string{"name"})
}
