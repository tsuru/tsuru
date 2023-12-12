// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tag

import (
	"context"

	tagTypes "github.com/tsuru/tsuru/types/tag"
	"google.golang.org/grpc"
)

var _ tagTypes.TagServiceClient = &noopTagClient{}

type noopTagClient struct{}

func (*noopTagClient) Validate(ctx context.Context, in *tagTypes.TagValidationRequest, opts ...grpc.CallOption) (*tagTypes.ValidationResponse, error) {
	return &tagTypes.ValidationResponse{Valid: true}, nil
}
