// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestLogWriter(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, string(data))
	c.Assert(logs[0].Source, check.Equals, "tsuru")
}

func (s *S) TestLogWriterCustomSource(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)

	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name, Source: "cool-test"}
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, string(data))
	c.Assert(logs[0].Source, check.Equals, "cool-test")
}

func (s *S) TestLogWriterShouldReturnTheDataSize(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(data))
}

func (s *S) TestLogWriterAsync(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	writer.Async()
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	writer.Close()
	err = writer.Wait(5 * time.Second)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs[0].Message, check.Equals, "ble")
	c.Assert(logs[0].Source, check.Equals, "tsuru")
}

func (s *S) TestLogWriterAsyncTimeout(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	writer.Async()
	err = writer.Wait(0)
	c.Assert(err, check.ErrorMatches, "timeout waiting for writer to finish")
	writer.Close()
	err = writer.Wait(10 * time.Second)
	c.Assert(err, check.IsNil)
}

func (s *S) TestLogWriterAsyncCopySlice(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	writer.Async()
	for i := 0; i < 100; i++ {
		data := []byte("ble")
		_, err = writer.Write(data)
		data[0] = 'X'
		c.Assert(err, check.IsNil)
	}
	writer.Close()
	err = writer.Wait(5 * time.Second)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 100,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 100)
	for i := 0; i < 100; i++ {
		c.Assert(logs[i].Message, check.Equals, "ble")
		c.Assert(logs[i].Source, check.Equals, "tsuru")
	}
}

func (s *S) TestLogWriterAsyncCloseWritingStress(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	writeFn := func(writer *LogWriter) {
		for i := 0; i < 100; i++ {
			data := []byte("ble")
			_, err := writer.Write(data)
			c.Assert(err, check.IsNil)
		}
	}
	for i := 0; i < 100; i++ {
		writer := LogWriter{AppName: a.Name}
		writer.Async()
		go writeFn(&writer)
		go writeFn(&writer)
		go writer.Close()
		err := writer.Wait(10 * time.Second)
		c.Assert(err, check.IsNil)
	}
}

func (s *S) TestLogWriterAsyncWriteClosed(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	writer.Async()
	writer.Close()
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	err = writer.Wait(5 * time.Second)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}

func (s *S) TestLogWriterWriteClosed(c *check.C) {
	appsCollection, err := storagev2.AppsCollection()
	c.Assert(err, check.IsNil)

	a := appTypes.App{Name: "down"}
	_, err = appsCollection.InsertOne(context.TODO(), a)
	c.Assert(err, check.IsNil)
	defer appsCollection.DeleteOne(context.TODO(), mongoBSON.M{"name": a.Name})
	writer := LogWriter{AppName: a.Name}
	writer.Close()
	data := []byte("ble")
	_, err = writer.Write(data)
	c.Assert(err, check.IsNil)
	err = writer.Wait(5 * time.Second)
	c.Assert(err, check.IsNil)
	instance := appTypes.App{}
	err = appsCollection.FindOne(context.TODO(), mongoBSON.M{"name": a.Name}).Decode(&instance)
	c.Assert(err, check.IsNil)
	logs, err := LastLogs(context.TODO(), &instance, servicemanager.LogService, appTypes.ListLogArgs{
		Limit: 1,
	})
	c.Assert(err, check.IsNil)
	c.Assert(logs, check.HasLen, 0)
}
