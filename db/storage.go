// Package db encapsulates tsuru connection with MongoDB.
//
// It uses data from configuration file to connect to the database, and store
// the connection in the Session variable.
package db

import (
	"labix.org/v2/mgo"
	"sync"
)

// Session stores the current connection with the database.
var Session *Storage

// Storage holds the connection with the database.
type Storage struct {
	collections map[string]*mgo.Collection
	session     *mgo.Session
	dbname      string
	sync.RWMutex
}

// Open dials to the MongoDB database.
//
// addr is a MongoDB connection URI.
//
// This function returns a pointer to a Storage, or a non-nil error in case of
// any failure.
func Open(addr, dbname string) (*Storage, error) {
	session, err := mgo.Dial(addr)
	if err != nil {
		return nil, err
	}
	s := &Storage{
		session:     session,
		collections: make(map[string]*mgo.Collection),
		dbname:      dbname,
	}
	return s, nil
}

// Close closes the connection.
//
// You can take advantage of defer statement, and write code that look like this:
//
//     st, err := Open("localhost:27017", "tsuru")
//     if err != nil {
//         panic(err)
//     }
//     defer st.Close()
func (s *Storage) Close() {
	s.session.Close()
}

// getCollection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) getCollection(name string) *mgo.Collection {
	s.RLock()
	collection, ok := s.collections[name]
	s.RUnlock()

	if !ok {
		collection = s.session.DB(s.dbname).C(name)
		s.Lock()
		s.collections[name] = collection
		s.Unlock()
	}
	return collection
}

// Apps returns the apps collection from MongoDB.
func (s *Storage) Apps() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.getCollection("apps")
	c.EnsureIndex(nameIndex)
	return c
}

// Services returns the services collection from MongoDB.
func (s *Storage) Services() *mgo.Collection {
	c := s.getCollection("services")
	return c
}

// ServiceInstances returns the services_instances collection from MongoDB.
func (s *Storage) ServiceInstances() *mgo.Collection {
	return s.getCollection("service_instances")
}

// Users returns the users collection from MongoDB.
func (s *Storage) Users() *mgo.Collection {
	emailIndex := mgo.Index{Key: []string{"email"}, Unique: true}
	c := s.getCollection("users")
	c.EnsureIndex(emailIndex)
	return c
}

// Teams returns the teams collection from MongoDB.
func (s *Storage) Teams() *mgo.Collection {
	return s.getCollection("teams")
}
