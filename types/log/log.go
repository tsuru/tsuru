package log

import (
	"context"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/types/auth"
)

type LogType string

var (
	LogTypeApp = LogType("app")
	LogTypeJob = LogType("job")
)

type LogabbleObject interface {
	GetName() string
	GetPool() string
}

type LogWatcher interface {
	Chan() <-chan LogEntry
	Close()
}

type LogService interface {
	Enqueue(entry *LogEntry) error
	Add(name, tsuruType string, message, source, unit string) error
	List(ctx context.Context, args ListLogArgs) ([]LogEntry, error)
	Watch(ctx context.Context, args ListLogArgs) (LogWatcher, error)
}

type LogServiceProvision interface {
	Provision(name, tsuruType string) error
	CleanUp(name, tsuruType string) error
}

type LogServiceInstance interface {
	Instance() LogService
}

type ListLogArgs struct {
	Name         string
	Type         string
	Source       string
	Units        []string
	Limit        int
	InvertSource bool
	Token        auth.Token
}

type LogEntry struct {
	MongoID bson.ObjectId `bson:"_id,omitempty" json:"-"`
	Date    time.Time
	Message string
	Source  string
	Name    string
	Type    string
	Unit    string
}
