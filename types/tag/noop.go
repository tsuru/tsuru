// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tag

import (
	"context"

	"google.golang.org/grpc"
)

var _ TagServiceClient = &noopTagClient{}

type noopTagClient struct{}

func (*noopTagClient) Validate(ctx context.Context, in *TagValidationRequest, opts ...grpc.CallOption) (*ValidationResponse, error) {
	return &ValidationResponse{Valid: true}, nil
}
