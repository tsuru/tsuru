package volume

import (
	"os"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"

	_ "github.com/tsuru/tsuru/storage/mongodb"
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	tearDown()
	os.Exit(code)
}

func setup() {
	otherProv := otherProvisioner{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("other", func() (provision.Provisioner, error) {
		return &otherProv, nil
	})
	setupConfig("")
}

func setupConfig(data string) {
	config.ReadConfigBytes([]byte(data))
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "tsuru_volume_test")
}

func tearDown() {
	err := storagev2.ClearAllCollections(nil)
	if err != nil {
		panic(err)
	}
}
