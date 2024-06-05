package storagev2

import (
	"context"
	"net/url"
	"strings"
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
	client       *mongo.Client
	databaseName string
)

func Connect() error {
	var uri string
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	uri, databaseName = dbConfig()

	client, err = mongo.Connect(
		ctx,
		options.Client().
			ApplyURI(uri).
			SetAppName("tsurud"),
	)
	if err != nil {
		return err
	}

	return nil
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

func Collection(name string) *mongo.Collection {
	return client.Database(databaseName).Collection(name, options.Collection())
}
