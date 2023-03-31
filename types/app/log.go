package app

import (
	"context"
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
	List(ctx context.Context, args ListLogArgs) ([]Applog, error)
	Watch(ctx context.Context, args ListLogArgs) (LogWatcher, error)
}

type AppLogServiceProvision interface {
	Provision(appName string) error
	CleanUp(appname string) error
}

type AppLogServiceInstance interface {
	Instance() AppLogService
}

type ListLogArgs struct {
	Name      string
	Type	  string
	Source       string
	Units        []string
	Limit        int
	InvertSource bool
	Token        auth.Token
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
