package kubernetes

import (
	"log"
	"os"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	os.Exit(code)
}

func setup() {
	setupConfig()

	storagev2.Reset()
	err := storagev2.ClearAllCollections(nil)
	if err != nil {
		log.Fatal("failed to clear storage collections", err)
	}
}

func setupConfig() {
	config.Set("log:disable-syslog", true)
	config.Set("auth:hash-cost", bcrypt.MinCost)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_kubernetes_tests_s")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queue_provision_kubernetes_tests")
	config.Set("queue:mongo-polling-interval", 0.01)
	config.Set("routers:fake:type", "fake")
	config.Set("routers:fake:default", true)
}
