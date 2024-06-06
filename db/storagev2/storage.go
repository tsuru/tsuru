package storagev2

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tsuru/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultDatabaseURL  = "mongodb://127.0.0.1:27017"
	DefaultDatabaseName = "tsuru"
)

var (
	client       atomic.Pointer[mongo.Client]
	databaseName string
)

func Reset() {
	client.Store(nil)
}

func connect() (*mongo.Client, error) {
	var uri string

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	uri, databaseName = dbConfig()

	connectedClient, err := mongo.Connect(
		ctx,
		options.Client().
			ApplyURI(uri).
			SetAppName("tsurud"),
	)
	if err != nil {
		return nil, err
	}

	client.Store(connectedClient)

	return connectedClient, nil
}

func dbConfig() (string, string) {
	uri, _ := config.GetString("database:url")
	if uri == "" {
		uri = DefaultDatabaseURL
	}

	if !strings.HasPrefix(uri, "mongodb://") {
		uri = "mongodb://" + uri
	}

	uriParsed, _ := url.Parse(uri)

	if uriParsed.Path == "" {
		uriParsed.Path = "/"
	}

	dbname, _ := config.GetString("database:name")
	if dbname == "" {
		dbname = DefaultDatabaseName
	}

	return uriParsed.String(), dbname
}

func Collection(name string) (*mongo.Collection, error) {
	connectedClient := client.Load()
	if connectedClient == nil {
		var err error
		connectedClient, err = connect()
		if err != nil {
			return nil, err
		}
	}
	return connectedClient.Database(databaseName).Collection(name, options.Collection()), nil
}
