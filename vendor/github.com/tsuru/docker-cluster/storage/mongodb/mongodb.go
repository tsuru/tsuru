// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/docker-cluster/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type mongodbStorage struct {
	session *mgo.Session
	dbName  string
}

func (s *mongodbStorage) StoreContainer(container, host string) error {
	coll := s.getColl("containers")
	defer coll.Database.Session.Close()
	_, err := coll.UpsertId(container, bson.M{"$set": bson.M{"host": host}})
	return err
}

func (s *mongodbStorage) RetrieveContainer(container string) (string, error) {
	coll := s.getColl("containers")
	defer coll.Database.Session.Close()
	dbContainer := struct {
		Host string
	}{}
	err := coll.Find(bson.M{"_id": container}).One(&dbContainer)
	if err != nil {
		if err == mgo.ErrNotFound {
			return "", storage.ErrNoSuchContainer
		}
		return "", err
	}
	return dbContainer.Host, nil
}

func (s *mongodbStorage) RemoveContainer(container string) error {
	coll := s.getColl("containers")
	defer coll.Database.Session.Close()
	return coll.Remove(bson.M{"_id": container})
}

func (s *mongodbStorage) RetrieveContainers() ([]cluster.Container, error) {
	coll := s.getColl("containers")
	defer coll.Database.Session.Close()
	var containers []cluster.Container
	err := coll.Find(nil).All(&containers)
	return containers, err
}

func (s *mongodbStorage) StoreImage(repo, id, host string) error {
	coll := s.getColl("images_history")
	defer coll.Database.Session.Close()
	_, err := coll.UpsertId(repo, bson.M{
		"$addToSet": bson.M{"history": bson.D([]bson.DocElem{
			// Order is important for $addToSet!
			{Name: "node", Value: host}, {Name: "imageid", Value: id},
		})},
		"$set": bson.M{"lastnode": host, "lastid": id},
	})
	return err
}

func (s *mongodbStorage) SetImageDigest(repo, digest string) error {
	coll := s.getColl("images_history")
	defer coll.Database.Session.Close()
	return coll.UpdateId(repo, bson.M{"$set": bson.M{"lastdigest": digest}})
}

func (s *mongodbStorage) RetrieveImage(repo string) (cluster.Image, error) {
	coll := s.getColl("images_history")
	defer coll.Database.Session.Close()
	var image cluster.Image
	err := coll.FindId(repo).One(&image)
	if err != nil {
		if err == mgo.ErrNotFound {
			return image, storage.ErrNoSuchImage
		}
		return image, err
	}
	if len(image.History) == 0 {
		return image, storage.ErrNoSuchImage
	}
	return image, nil
}

func (s *mongodbStorage) RemoveImage(repo, id, host string) error {
	coll := s.getColl("images_history")
	defer coll.Database.Session.Close()
	return coll.UpdateId(repo, bson.M{"$pull": bson.M{"history": bson.M{"node": host, "imageid": id}}})
}

func (s *mongodbStorage) RetrieveImages() ([]cluster.Image, error) {
	coll := s.getColl("images_history")
	defer coll.Database.Session.Close()
	var images []cluster.Image
	err := coll.Find(nil).All(&images)
	return images, err
}

func (s *mongodbStorage) StoreNode(node cluster.Node) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	err := coll.Insert(node)
	if mgo.IsDup(err) {
		return storage.ErrDuplicatedNodeAddress
	}
	return err
}

func (s *mongodbStorage) LockNodeForHealing(address string, isFailure bool, timeout time.Duration) (bool, error) {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	now := time.Now().UTC()
	until := now.Add(timeout)
	setOperation := bson.M{"$set": bson.M{"healing": bson.M{"lockeduntil": until, "isfailure": isFailure}}}
	err := coll.Update(
		bson.M{"_id": address, "healing.lockeduntil": nil},
		setOperation)
	if err == mgo.ErrNotFound {
		var dbNode cluster.Node
		err = coll.Find(bson.M{"_id": address}).One(&dbNode)
		if dbNode.Healing.LockedUntil.After(now) {
			return false, nil
		}
		err = coll.Update(bson.M{
			"_id": address,
			"healing.lockeduntil": dbNode.Healing.LockedUntil,
		}, setOperation)
		if err == mgo.ErrNotFound {
			return false, nil
		}
	}
	return err == nil, err
}

func (s *mongodbStorage) ExtendNodeLock(address string, timeout time.Duration) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	now := time.Now().UTC()
	until := now.Add(timeout)
	return coll.Update(
		bson.M{"_id": address},
		bson.M{"$set": bson.M{"healing.lockeduntil": until}})
}

func (s *mongodbStorage) UnlockNode(address string) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	return coll.Update(
		bson.M{"_id": address},
		bson.M{"$set": bson.M{"healing": nil}})
}

func (s *mongodbStorage) RetrieveNodesByMetadata(metadata map[string]string) ([]cluster.Node, error) {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	query := bson.M{}
	for key, value := range metadata {
		query["metadata."+key] = value
	}
	var nodes []cluster.Node
	err := coll.Find(query).All(&nodes)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *mongodbStorage) RetrieveNodes() ([]cluster.Node, error) {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	var nodes []cluster.Node
	err := coll.Find(nil).All(&nodes)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *mongodbStorage) RetrieveNode(address string) (cluster.Node, error) {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	var node cluster.Node
	err := coll.FindId(address).One(&node)
	if err == mgo.ErrNotFound {
		return cluster.Node{}, storage.ErrNoSuchNode
	}
	return node, err
}

func (s *mongodbStorage) UpdateNode(node cluster.Node) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	err := coll.UpdateId(node.Address, node)
	if err == mgo.ErrNotFound {
		return storage.ErrNoSuchNode
	}
	return err
}

func (s *mongodbStorage) RemoveNode(address string) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	err := coll.Remove(bson.M{"_id": address})
	if err == mgo.ErrNotFound {
		return storage.ErrNoSuchNode
	}
	return err
}

func (s *mongodbStorage) RemoveNodes(addresses []string) error {
	coll := s.getColl("nodes")
	defer coll.Database.Session.Close()
	change, err := coll.RemoveAll(bson.M{"_id": bson.M{"$in": addresses}})
	if err != nil {
		return err
	}
	if change.Removed == 0 {
		return storage.ErrNoSuchNode
	}
	return nil
}

func (s *mongodbStorage) getColl(name string) *mgo.Collection {
	session := s.session.Copy()
	return session.DB(s.dbName).C(name)
}

func Mongodb(addr, dbName string) (cluster.Storage, error) {
	dialInfo, err := mgo.ParseURL(addr)
	if err != nil {
		return nil, err
	}
	dialInfo.FailFast = true
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	session.SetSyncTimeout(10 * time.Second)
	session.SetSocketTimeout(1 * time.Minute)
	storage := mongodbStorage{
		session: session,
		dbName:  dbName,
	}
	return &storage, nil
}
