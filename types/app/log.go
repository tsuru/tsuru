package app

import (
	"time"

	"github.com/globalsign/mgo/bson"
)

type LogWatcher interface {
	Chan() <-chan Applog
	Close()
}

type AppLogService interface {
	Enqueue(entry *Applog) error
	Add(appName, message, source, unit string) error
	List(args ListLogArgs) ([]Applog, error)
	Watch(appName, source, unit string) (LogWatcher, error)
}

type AppLogStorage interface {
	Insert(msgs ...Applog) error
	List(args ListLogArgs) ([]Applog, error)
	Watch(appName, source, unit string) (LogWatcher, error)
}

type ListLogArgs struct {
	AppName       string
	Source        string
	Unit          string
	Limit         int
	InvertFilters bool
}

// Applog represents a log entry.
type Applog struct {
	MongoID bson.ObjectId `bson:"_id,omitempty" json:"-"`
	Date    time.Time
	Message string
	Source  string
	AppName string
	Unit    string
}
