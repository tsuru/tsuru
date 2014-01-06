package storage

import (
    "labix.org/v2/mgo"
    "sync"
    "time"
)

var (
    conn   = make(map[string]*session) // pool of connections
    mut    sync.RWMutex                // for pool thread safety
    ticker *time.Ticker                // for garbage collection
)

type session struct {
    s    *mgo.Session
    used time.Time
}

const period time.Duration = 7 * 24 * time.Hour

// Storage holds the connection with the database.
type Storage struct {
    session *mgo.Session
    dbname  string
}

// Collection represents a database collection. It embeds mgo.Collection for
// operations, and holds a session to MongoDB. The user may close the session
// using the method close.
type Collection struct {
    *mgo.Collection
}

// Close closes the session with the database.
func (c *Collection) Close() {
    c.Collection.Database.Session.Close()
}

func open(addr, dbname string) (*Storage, error) {
    sess, err := mgo.Dial(addr)
    if err != nil {
        return nil, err
    }
    copy := sess.Clone()
    storage := &Storage{session: copy, dbname: dbname}
    mut.Lock()
    conn[addr] = &session{s: sess, used: time.Now()}
    mut.Unlock()
    return storage, nil
}
// Open dials to the MongoDB database, and return the connection (represented
// by the type Storage).
//
// addr is a MongoDB connection URI, and dbname is the name of the database.
//
// This function returns a pointer to a Storage, or a non-nil error in case of
// any failure.
func Open(addr, dbname string) (storage *Storage, err error) {
    defer func() {
        if r := recover(); r != nil {
            storage, err = open(addr, dbname)
        }
    }()
    mut.RLock()
    if session, ok := conn[addr]; ok {
        mut.RUnlock()
        if err = session.s.Ping(); err == nil {
            mut.Lock()
            session.used = time.Now()
            conn[addr] = session
            mut.Unlock()
            copy := session.s.Clone()
            return &Storage{copy, dbname}, nil
        }
        return open(addr, dbname)
    }
    mut.RUnlock()
    return open(addr, dbname)
}

// Close closes the storage, releasing the connection.
func (s *Storage) Close() {
    s.session.Close()
}

// Collection returns a collection by its name.
//
// If the collection does not exist, MongoDB will create it.
func (s *Storage) Collection(name string) *Collection {
    return &Collection{s.session.DB(s.dbname).C(name)}
}

func init() {
    ticker = time.NewTicker(time.Hour)
    go retire(ticker)
}

// retire retires old connections
func retire(t *time.Ticker) {
    for _ = range t.C {
        now := time.Now()
        var old []string
        mut.RLock()
        for k, v := range conn {
            if now.Sub(v.used) >= period {
                old = append(old, k)
            }
        }
        mut.RUnlock()
        mut.Lock()
        for _, c := range old {
            conn[c].s.Close()
            delete(conn, c)
        }
        mut.Unlock()
    }
}
