// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package tag

import (
	context "context"

	grpc "google.golang.org/grpc"
)

var _ TagServiceClient = &MockServiceTagServiceClient{}

type MockServiceTagServiceClient struct {
	OnValidate func(in *TagValidationRequest) (*ValidationResponse, error)
}

func (m *MockServiceTagServiceClient) Validate(ctx context.Context, in *TagValidationRequest, opts ...grpc.CallOption) (*ValidationResponse, error) {
	return m.OnValidate(in)
}
