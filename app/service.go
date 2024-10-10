// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"fmt"
	"strings"

	uuid "github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	imgTypes "github.com/tsuru/tsuru/types/app/image"
	routerTypes "github.com/tsuru/tsuru/types/router"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type appService struct{}

func (a *appService) GetByName(ctx context.Context, name string) (*appTypes.App, error) {
	return GetByName(ctx, name)
}

func (a *appService) GetAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	return GetAddresses(ctx, app)
}

func (a *appService) GetRegistry(ctx context.Context, app *appTypes.App) (imgTypes.ImageRegistry, error) {
	return GetRegistry(ctx, app)
}

func (a *appService) GetInternalBindableAddresses(ctx context.Context, app *appTypes.App) ([]string, error) {
	prov, err := getProvisioner(ctx, app)
	if err != nil {
		return nil, err
	}
	interAppProv, ok := prov.(provision.InterAppProvisioner)
	if !ok {
		return nil, nil
	}
	addrs, err := interAppProv.InternalAddresses(ctx, app)
	if err != nil {
		return nil, err
	}
	var addresses []string
	for _, addr := range addrs {
		// version addresses are so volatile, they change after every deploy, we don't use them to bind process
		if addr.Version != "" {
			continue
		}
		addresses = append(addresses, fmt.Sprintf("%s://%s:%d", strings.ToLower(addr.Protocol), addr.Domain, addr.Port))
	}
	return addresses, nil
}

// GetUUID returns the app v4 UUID. An UUID will be generated
// if it does not exist.
func (a *appService) EnsureUUID(ctx context.Context, app *appTypes.App) (string, error) {
	if app.UUID != "" {
		return app.UUID, nil
	}
	uuidV4, err := uuid.NewV4()
	if err != nil {
		return "", errors.WithMessage(err, "failed to generate uuid v4")
	}
	collection, err := storagev2.AppsCollection()
	if err != nil {
		return "", err
	}
	_, err = collection.UpdateOne(ctx, mongoBSON.M{"name": app.Name}, mongoBSON.M{"$set": mongoBSON.M{"uuid": uuidV4.String()}})
	if err != nil {
		return "", err
	}
	app.UUID = uuidV4.String()
	return app.UUID, nil
}

func (a *appService) List(ctx context.Context, filter *appTypes.Filter) ([]*appTypes.App, error) {
	var f *Filter
	if filter != nil {
		f = func(f Filter) *Filter { return &f }(Filter(*filter))
	}
	return List(ctx, f)
}

func (a *appService) GetHealthcheckData(ctx context.Context, app *appTypes.App) (routerTypes.HealthcheckData, error) {
	version, err := servicemanager.AppVersion.LatestSuccessfulVersion(ctx, app)
	if err != nil {
		if err == appTypes.ErrNoVersionsAvailable {
			err = nil
		}
		return routerTypes.HealthcheckData{}, err
	}
	yamlData, err := version.TsuruYamlData()
	if err != nil {
		return routerTypes.HealthcheckData{}, err
	}

	prov, err := pool.GetProvisionerForPool(ctx, app.Pool)
	if err != nil {
		return routerTypes.HealthcheckData{}, err
	}
	if hcProv, ok := prov.(provision.HCProvisioner); ok {
		if hcProv.HandlesHC() {
			return routerTypes.HealthcheckData{
				TCPOnly: true,
			}, nil
		}
	}
	return yamlData.ToRouterHC(), nil
}

func AppService() (appTypes.AppService, error) {
	return &appService{}, nil
}
