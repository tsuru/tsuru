// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package saml

import (
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
)

var (
	ErrRequestNotFound = &errors.ValidationError{Message: "request not found or expired"}
)

type request struct {
	ID       string    `json:"id"`
	Creation time.Time `json:"creation"`
	Expires  time.Time `json:"expires"`
	Email    string    `json:"email"`
	Authed   bool      `json:"authed"`
}

func (r *request) expireTime() time.Duration {
	if sec, err := config.GetInt("auth:saml:request-expire-seconds"); err == nil {
		return time.Duration(int64(sec) * int64(time.Second))
	}
	return 3 * 60 * time.Second
}

func (r *request) Create(ar *AuthnRequestData) (*request, error) {
	if ar == nil {
		return nil, &errors.ValidationError{Message: "AuthnRequest is nil"}
	}
	if ar.ID == "" {
		return nil, &errors.ValidationError{Message: "Impossible get ID from AuthnRequest"}
	}
	r.ID = ar.ID
	r.Creation = time.Now()
	r.Expires = time.Now().Add(r.expireTime())
	r.Authed = false
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.SAMLRequests().Insert(r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (request *request) getById(id string) error {
	removeOldRequests()

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.SAMLRequests().Find(bson.M{"id": id}).One(request)
	if err != nil {
		return ErrRequestNotFound
	}
	return nil
}

func (req *request) Update() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.SAMLRequests().Update(bson.M{"id": req.ID}, req)
}

func (req *request) Remove() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.SAMLRequests().RemoveAll(bson.M{"id": req.ID})
	return err
}

func removeOldRequests() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.SAMLRequests().RemoveAll(bson.M{"expires": bson.M{"$lt": time.Now()}, "authed": false})
	return err
}
