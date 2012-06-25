package db

import (
	"labix.org/v2/mgo"
	"sync"
)

type Storage struct {
	collections map[string]*mgo.Collection
	session     *mgo.Session
	dbname      string
	sync.RWMutex
}

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

func (s *Storage) Close() {
	s.session.Close()
}

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

func (s *Storage) Apps() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.getCollection("apps")
	c.EnsureIndex(nameIndex)
	return c
}

func (s *Storage) Services() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.getCollection("services")
	c.EnsureIndex(nameIndex)
	return c
}

func (s *Storage) ServiceApps() *mgo.Collection {
	return s.getCollection("service_apps")
}

func (s *Storage) ServiceTypes() *mgo.Collection {
	return s.getCollection("service_types")
}

func (s *Storage) Units() *mgo.Collection {
	return s.getCollection("units")
}

func (s *Storage) Users() *mgo.Collection {
	emailIndex := mgo.Index{Key: []string{"email"}, Unique: true}
	c := s.getCollection("users")
	c.EnsureIndex(emailIndex)
	return c
}

func (s *Storage) Teams() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	c := s.getCollection("teams")
	c.EnsureIndex(nameIndex)
	return c
}
