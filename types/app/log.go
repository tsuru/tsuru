package app

import (
	"context"
	"time"

	"github.com/globalsign/mgo/bson"
	logTypes "github.com/tsuru/tsuru/types/log"
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
	Name         string
	Type         logTypes.LogType
	Source       string
	Units        []string
	Limit        int
	InvertSource bool
}

// Applog represents a log entry.
type Applog struct {
	MongoID bson.ObjectId `bson:"_id,omitempty" json:"-"`
	Date    time.Time
	Message string
	Source  string
	Name    string
	Type    logTypes.LogType
	Unit    string
}
