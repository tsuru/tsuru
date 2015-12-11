// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package saml

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2/bson"
)

var (
	requestExpire            time.Duration
	defaultRequestExpiration = 3 * 60 * time.Second
	ErrRequestNotFound       = &errors.ValidationError{Message: "request not found or expired"}
)

type Request struct {
	ID       string    `json:"id"`
	Creation time.Time `json:"creation"`
	Expires  time.Time `json:"expires"`
	Email    string    `json:"email"`
	Authed   bool      `json:"authed"`
}

func loadExpireTime() error {
	if requestExpire == 0 {
		var err error
		var min int
		if min, err = config.GetInt("auth:saml:request-expire-seconds"); err == nil {
			requestExpire = time.Duration(int64(min) * int64(time.Second))
		} else {
			requestExpire = defaultRequestExpiration
		}

	}
	return nil
}

func (r *Request) GetExpireTimeOut() int {
	sec, err := config.GetInt("auth:saml:request-expire-seconds")

	if err != nil {
		sec, _ = strconv.Atoi(fmt.Sprintf("%d", defaultRequestExpiration/time.Second))
		log.Debugf("auth:saml:request-expire-seconds not found using default: %s", sec)
	}
	return sec
}

func (r *Request) Create(ar *AuthnRequestData) (*Request, error) {
	loadExpireTime()

	if ar == nil {
		return nil, &errors.ValidationError{Message: "AuthnRequest is nil"}
	}
	if ar.ID == "" {
		return nil, &errors.ValidationError{Message: "Impossible get ID from AuthnRequest"}
	}

	r.ID = ar.ID
	r.Creation = time.Now()
	r.Expires = time.Now().Add(requestExpire)
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

func GetRequestById(id string) (Request, error) {
	removeOldTRequests()

	request := Request{}

	conn, err := db.Conn()
	if err != nil {
		return request, err
	}
	defer conn.Close()
	err = conn.SAMLRequests().Find(bson.M{"id": id}).One(&request)
	if err != nil {
		return request, ErrRequestNotFound
	}
	return request, nil
}

func (req *Request) GetEmail() string {
	return req.Email
}

func (req *Request) SetAuth(auth bool) {
	req.Authed = true
}

func (req *Request) SetEmail(email string) {
	req.Email = email
}

func (req *Request) Update() error {

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.SAMLRequests().Update(bson.M{"id": req.ID}, req)
}

func (req *Request) Remove() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.SAMLRequests().RemoveAll(bson.M{"id": req.ID})
	return err

}

func removeOldTRequests() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.SAMLRequests().RemoveAll(bson.M{"expires": bson.M{"$lt": time.Now()}, "authed": false})
	return err
}
