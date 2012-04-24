package db

import (
	"launchpad.net/mgo"
	"sync"
)

type Storage struct {
	collections map[string]*mgo.Collection
	session     *mgo.Session
	sync.Mutex
}

func Open(addr string) (*Storage, error) {
	session, err := mgo.Dial(addr)
	if err != nil {
		return nil, err
	}
	s := &Storage{
		session:     session,
		collections: make(map[string]*mgo.Collection),
	}
	return s, nil
}

func (s *Storage) Close() {
	s.session.Close()
}

func (s *Storage) getCollection(name string) *mgo.Collection {
	collection, ok := s.collections[name]
	if !ok {
		collection = s.session.DB("tsuru").C(name)
		s.collections[name] = collection
	}
	return collection
}

func (s *Storage) Apps() *mgo.Collection {
	return s.getCollection("apps")
}

func (s *Storage) Services() *mgo.Collection {
	return s.getCollection("services")
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
