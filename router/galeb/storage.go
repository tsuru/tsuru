// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package galeb

import "gopkg.in/mgo.v2/bson"

type galebCNameData struct {
	CName         string
	VirtualHostId string
}

type galebRealData struct {
	Real      string
	BackendId string
}

type galebData struct {
	Name          string `bson:"_id"`
	BackendPoolId string
	RootRuleId    string
	VirtualHostId string
	CNames        []galebCNameData
	Reals         []galebRealData
}

func (g *galebData) save() error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Insert(g)
}

func (g *galebData) addReal(address, backendId string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(g.Name, bson.M{"$push": bson.M{
		"reals": bson.M{"real": address, "backendid": backendId},
	}})
}

func (g *galebData) removeReal(address string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(g.Name, bson.M{"$pull": bson.M{
		"reals": bson.M{"real": address},
	}})
}

func (g *galebData) addCName(cname, virtualHostId string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(g.Name, bson.M{"$push": bson.M{
		"cnames": bson.M{"cname": cname, "virtualhostid": virtualHostId},
	}})
}

func (g *galebData) removeCName(cname string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.UpdateId(g.Name, bson.M{"$pull": bson.M{
		"cnames": bson.M{"cname": cname},
	}})
}

func (g *galebData) remove() error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.RemoveId(g.Name)
}

func getGalebData(name string) (*galebData, error) {
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var result galebData
	err = coll.Find(bson.M{"_id": name}).One(&result)
	return &result, err
}
