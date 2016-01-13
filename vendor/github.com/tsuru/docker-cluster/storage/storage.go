// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package storage provides some implementations of the Storage interface,
// defined in the cluster package.
package storage

import (
	"errors"
)

var (
	ErrNoSuchNode            = errors.New("No such node in storage")
	ErrNoSuchContainer       = errors.New("No such container in storage")
	ErrNoSuchImage           = errors.New("No such image in storage")
	ErrDuplicatedNodeAddress = errors.New("Node address shouldn't repeat")
)
