// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Finds tsuru applications which deploys count % 20 == 0 |||| this is wrong! if count is 30 % 20 will be 10 and the app still needs a flatten!
// and flatten their filesystems in order to avoid aufs performance bottlenecks.
package docker

import (
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"labix.org/v2/mgo/bson"
)

func imagesToFlatten() []string {
	var apps []app.App
	conn, err := db.Conn()
	if err != nil {
		log.Fatalf("Caught error while connecting with database: %s", err.Error())
		panic(err)
		return nil
	}
	filter := bson.M{"deploys": bson.M{"$mod": []int{20, 0}}}
	if err := conn.Apps().Find(filter).Select(bson.M{"name": 1}).All(&apps); err != nil {
		log.Fatalf("Caught error while getting apps from database: %s", err.Error())
		panic(err)
		return nil
	}
	images := make([]string, len(apps))
	for i, a := range apps {
		images[i] = assembleImageName(a.GetName())
	}
	return images
}
