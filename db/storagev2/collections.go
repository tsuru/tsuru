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

func JobsCollection() (*mongo.Collection, error) {
	return Collection("jobs")
}

func TokensCollection() (*mongo.Collection, error) {
	return Collection("tokens")
}

func PasswordTokensCollection() (*mongo.Collection, error) {
	return Collection("password_tokens")
}

func UsersCollection() (*mongo.Collection, error) {
	return Collection("users")
}

func TeamTokensCollection() (*mongo.Collection, error) {
	return Collection("team_tokens")
}

func TeamsCollection() (*mongo.Collection, error) {
	return Collection("teams")
}

func PlansCollection() (*mongo.Collection, error) {
	return Collection("plans")
}

func WebhookCollection() (*mongo.Collection, error) {
	return Collection("webhook")
}

func VolumesCollection() (*mongo.Collection, error) {
	return Collection("volumes")
}

func VolumeBindsCollection() (*mongo.Collection, error) {
	return Collection("volume_binds")
}

func TrackerCollection() (*mongo.Collection, error) {
	return Collection("tracker")
}

func OAuth2TokensCollection() (*mongo.Collection, error) {
	collectionName := getOAuthTokensCollectionName()
	return Collection(collectionName)
}
