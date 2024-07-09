// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagev2

import (
	"go.mongodb.org/mongo-driver/mongo"
)

func PoolCollection() (*mongo.Collection, error) {
	return Collection("pool")
}

func PoolConstraintsCollection() (*mongo.Collection, error) {
	return Collection("pool_constraints")
}

func EventsCollection() (*mongo.Collection, error) {
	return Collection("events")
}

func ServicesCollection() (*mongo.Collection, error) {
	return Collection("services")
}

func ServiceInstancesCollection() (*mongo.Collection, error) {
	return Collection("service_instances")
}

func RolesCollection() (*mongo.Collection, error) {
	return Collection("roles")
}

func PlatformImagesCollection() (*mongo.Collection, error) {
	return Collection("platform_images")
}
