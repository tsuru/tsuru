// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tag

import (
	"github.com/tsuru/config"
	tagTypes "github.com/tsuru/tsuru/types/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TagService() (tagTypes.TagServiceClient, error) {
	tagServiceAddr, _ := config.GetString("tag:service-addr")
	if tagServiceAddr != "" {
		conn, err := grpc.NewClient(tagServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}

		return tagTypes.NewTagServiceClient(conn), nil
	}
	return &noopTagClient{}, nil
}
