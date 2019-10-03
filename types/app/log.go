package app

import (
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/types/auth"
)

type LogWatcher interface {
	Chan() <-chan Applog
	Close()
}

type AppLogService interface {
	Enqueue(entry *Applog) error
	Add(appName, message, source, unit string) error
	List(args ListLogArgs) ([]Applog, error)
	Watch(appName, source, unit string, t auth.Token) (LogWatcher, error)
}

type AppLogServiceInstance interface {
	Instance() AppLogService
}

type AppLogStorage interface {
	InsertApp(appName string, msgs ...*Applog) error
	List(args ListLogArgs) ([]Applog, error)
	Watch(appName, source, unit string) (LogWatcher, error)
}

type ListLogArgs struct {
	AppName       string
	Source        string
	Unit          string
	Limit         int
	Level         int
	InvertFilters bool
	Token         auth.Token
}

// Applog represents a log entry.
type Applog struct {
	MongoID bson.ObjectId `bson:"_id,omitempty" json:"-"`
	Date    time.Time
	Message string
	Source  string
	AppName string
	Unit    string
	Level   int
}
