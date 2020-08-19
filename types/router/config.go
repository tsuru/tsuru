// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

type ConfigGetter interface {
	GetString(string) (string, error)
	GetInt(string) (int, error)
	GetFloat(string) (float64, error)
	GetBool(string) (bool, error)
	Get(string) (interface{}, error)
	Hash() (string, error)
}
