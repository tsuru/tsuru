// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scopedconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
)

const (
	defaultConfigName = ""
)

type ScopedConfig struct {
	coll          string
	name          string
	AllowedPools  []string
	AllowEmpty    bool
	ShallowMerge  bool
	Jsonfy        bool
	SliceAdd      bool
	AllowMapEmpty bool
	PtrNilIsEmpty bool
}

type scopedConfigEntry struct {
	Name string
	Pool string
	Val  bson.Raw
}

func FindScopedConfig(coll string) *ScopedConfig {
	return FindScopedConfigFor(coll, defaultConfigName)
}

func FindScopedConfigFor(coll, name string) *ScopedConfig {
	return &ScopedConfig{coll: fmt.Sprintf("scoped_%s", coll), name: name}
}

func FindAllScopedConfigNames(collName string) ([]string, error) {
	base := FindScopedConfig(collName)
	coll, err := base.collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var names []string
	err = coll.Find(nil).Distinct("name", &names)
	return names, err
}

func (n *ScopedConfig) GetName() string {
	return n.name
}

func (n *ScopedConfig) SetFieldAtomic(pool, name string, value interface{}) (bool, error) {
	coll, err := n.collection()
	if err != nil {
		return false, err
	}
	defer coll.Close()
	_, err = coll.Upsert(bson.M{
		"name": n.name,
		"pool": pool,
		"$or":  []bson.M{{"val." + name: ""}, {"val." + name: bson.M{"$exists": false}}},
	}, bson.M{"$set": bson.M{"val." + name: value}})
	if err == nil {
		return true, nil
	}
	if mgo.IsDup(err) {
		return false, nil
	}
	return false, err
}

func (n *ScopedConfig) SetField(pool, name string, value interface{}) error {
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.Upsert(bson.M{"name": n.name, "pool": pool}, bson.M{"$set": bson.M{"val." + name: value}})
	return err
}

func (n *ScopedConfig) SaveBase(val interface{}) error {
	return n.Save("", val)
}

func (n *ScopedConfig) Save(pool string, val interface{}) error {
	if reflect.TypeOf(val).Kind() == reflect.Ptr {
		val = reflect.ValueOf(val).Elem().Interface()
	}
	if reflect.TypeOf(val).Kind() != reflect.Struct {
		return errors.New("a struct type or pointer to a struct is required as value")
	}
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	if n.Jsonfy {
		var result map[string]interface{}
		var data []byte
		data, err = json.Marshal(val)
		if err != nil {
			return err
		}
		err = json.Unmarshal(data, &result)
		if err != nil {
			return err
		}
		val = interface{}(result)
	}
	_, err = coll.Upsert(bson.M{"name": n.name, "pool": pool}, bson.M{"name": n.name, "pool": pool, "val": val})
	return err
}

func (n *ScopedConfig) HasEntry(pool string) (bool, error) {
	coll, err := n.collection()
	if err != nil {
		return false, err
	}
	defer coll.Close()
	count, err := coll.Find(bson.M{"name": n.name, "pool": pool}).Count()
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

func (n *ScopedConfig) SaveMerge(pool string, val interface{}) error {
	if reflect.TypeOf(val).Kind() == reflect.Ptr {
		val = reflect.ValueOf(val).Elem().Interface()
	}
	if reflect.TypeOf(val).Kind() != reflect.Struct {
		return errors.New("a struct type or pointer to a struct is required as value")
	}
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	var poolValues scopedConfigEntry
	previousValue := reflect.New(reflect.ValueOf(val).Type())
	err = coll.Find(bson.M{"name": n.name, "pool": pool}).One(&poolValues)
	if err == nil {
		err = n.unmarshal(&poolValues.Val, previousValue.Interface())
		if err != nil {
			return err
		}
	} else if err != mgo.ErrNotFound {
		return err
	}
	_, err = n.mergeIntoInherited(previousValue.Elem(), reflect.ValueOf(val), false)
	if err != nil {
		return err
	}
	return n.Save(pool, previousValue.Elem().Interface())
}

func (n *ScopedConfig) LoadAll(allVal interface{}) error {
	return n.LoadPoolsMerge(nil, allVal, true, true)
}

func (n *ScopedConfig) LoadPools(filterPools []string, allVal interface{}) error {
	return n.LoadPoolsMerge(filterPools, allVal, true, true)
}

func (n *ScopedConfig) LoadPoolsMerge(filterPools []string, allVal interface{}, merge bool, includeDefault bool) error {
	allValValue := reflect.ValueOf(allVal)
	var isPtr bool
	if allValValue.Type().Kind() == reflect.Ptr {
		isPtr = true
		allValValue = allValValue.Elem()
	}
	if allValValue.Type().Kind() != reflect.Map ||
		allValValue.Type().Key().Kind() != reflect.String ||
		allValValue.Type().Elem().Kind() != reflect.Struct {
		return errors.Errorf("received object must be a map[string]<yourstruct>, received: %v", allValValue.Type())
	}
	if isPtr {
		allValValue.Set(reflect.MakeMap(allValValue.Type()))
	}
	if allValValue.IsNil() {
		return errors.Errorf("uninitialized map")
	}
	var defaultValues scopedConfigEntry
	var allPoolValues []scopedConfigEntry
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"name": n.name, "pool": ""}).One(&defaultValues)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	mapType := allValValue.Type().Elem()
	baseValue := reflect.New(mapType)
	baseVal := baseValue.Interface()
	if defaultValues.Val.Data != nil {
		err = n.unmarshal(&defaultValues.Val, baseVal)
		if err != nil {
			return err
		}
	}
	if includeDefault || defaultValues.Val.Data != nil {
		allValValue.SetMapIndex(reflect.ValueOf(""), baseValue.Elem())
	}
	if len(filterPools) == 0 {
		err = coll.Find(bson.M{"name": n.name, "pool": bson.M{"$ne": ""}}).All(&allPoolValues)
	} else {
		err = coll.Find(bson.M{"name": n.name, "pool": bson.M{"$in": filterPools}}).All(&allPoolValues)
	}
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	for i := range allPoolValues {
		if merge {
			baseValue = reflect.New(mapType)
			baseVal = baseValue.Interface()
			if defaultValues.Val.Data != nil {
				err = n.unmarshal(&defaultValues.Val, baseVal)
				if err != nil {
					return err
				}
			}
		}
		poolValue := reflect.New(mapType)
		poolVal := poolValue.Interface()
		err = n.unmarshal(&allPoolValues[i].Val, poolVal)
		if err != nil {
			return err
		}
		indexValue := reflect.ValueOf(allPoolValues[i].Pool)
		if merge {
			_, err = n.mergeInto(baseValue.Elem(), poolValue.Elem())
			if err != nil {
				return err
			}
			allValValue.SetMapIndex(indexValue, baseValue.Elem())
		} else {
			allValValue.SetMapIndex(indexValue, poolValue.Elem())
		}
	}
	return nil
}

func (n *ScopedConfig) Load(pool string, poolVal interface{}) error {
	return n.LoadWithBase(pool, nil, poolVal)
}

func (n *ScopedConfig) LoadBase(poolVal interface{}) error {
	return n.LoadWithBase("", nil, poolVal)
}

func (n *ScopedConfig) LoadWithBase(pool string, baseVal interface{}, poolVal interface{}) error {
	poolValue := reflect.ValueOf(poolVal)
	if poolValue.Type().Kind() != reflect.Ptr ||
		poolValue.Elem().Type().Kind() != reflect.Struct {
		return errors.New("received object must be a pointer to a struct")
	}
	var baseValue reflect.Value
	if baseVal == nil {
		baseValue = reflect.New(poolValue.Elem().Type())
		baseVal = baseValue.Interface()
	} else {
		baseValue = reflect.ValueOf(baseVal)
		if baseValue.Type().Kind() != reflect.Ptr {
			return errors.New("received object must be a pointer to a struct")
		}
		if poolValue.Elem().Type() != baseValue.Elem().Type() {
			return errors.New("received object must the same type")
		}
	}
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	var defaultValues, poolValues scopedConfigEntry
	err = coll.Find(bson.M{"name": n.name, "pool": ""}).One(&defaultValues)
	if err == nil {
		err = n.unmarshal(&defaultValues.Val, baseVal)
		if err != nil {
			return err
		}
	} else if err != mgo.ErrNotFound {
		return err
	}
	if pool == "" {
		poolValue.Elem().Set(baseValue.Elem())
		return nil
	}
	baseCopy := reflect.New(baseValue.Elem().Type())
	if defaultValues.Val.Data != nil {
		baseCopyVal := baseCopy.Interface()
		err = n.unmarshal(&defaultValues.Val, baseCopyVal)
		if err != nil {
			return err
		}
	}
	err = coll.Find(bson.M{"name": n.name, "pool": pool}).One(&poolValues)
	if err == nil {
		err = n.unmarshal(&poolValues.Val, poolVal)
		if err != nil {
			return err
		}
	} else if err != mgo.ErrNotFound {
		return err
	}
	_, err = n.mergeInto(baseCopy.Elem(), poolValue.Elem())
	if err != nil {
		return err
	}
	poolValue.Elem().Set(baseCopy.Elem())
	return nil
}

func (n *ScopedConfig) Remove(pool string) error {
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	return coll.Remove(bson.M{"name": n.name, "pool": pool})
}

func (n *ScopedConfig) RemoveField(pool, name string) error {
	coll, err := n.collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	name = strings.ToLower(name)
	err = coll.Update(bson.M{"name": n.name, "pool": pool}, bson.M{"$unset": bson.M{"val." + name: ""}})
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	return nil
}

func (n *ScopedConfig) unmarshal(raw *bson.Raw, dst interface{}) error {
	if !n.Jsonfy {
		return raw.Unmarshal(dst)
	}
	var mapresult map[string]interface{}
	err := raw.Unmarshal(&mapresult)
	if err != nil {
		return err
	}
	data, err := json.Marshal(mapresult)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func (n *ScopedConfig) mergeInto(base reflect.Value, pool reflect.Value) (merged bool, err error) {
	return n.mergeIntoInherited(base, pool, true)
}

func (n *ScopedConfig) mergeIntoInherited(base reflect.Value, pool reflect.Value, setInherited bool) (merged bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("error trying to merge items: %s", r)
		}
	}()
	switch base.Kind() {
	case reflect.Struct:
		if _, isTime := base.Interface().(time.Time); isTime {
			if !n.isEmpty(pool) {
				base.Set(pool)
			}
			break
		}
		numField := base.Type().NumField()
		for i := 0; i < numField; i++ {
			fieldType := base.Type().Field(i)
			isInherited := strings.HasSuffix(strings.ToLower(fieldType.Name), "inherited")
			if isInherited {
				continue
			}
			inheritedField, hasInherited := base.Type().FieldByNameFunc(func(name string) bool {
				return strings.EqualFold(name, fieldType.Name+"inherited")
			})
			if fieldType.PkgPath != "" && !fieldType.Anonymous {
				continue
			}
			f1Value := base.Field(i)
			f2Value := pool.Field(i)
			if n.ShallowMerge {
				if !n.isEmpty(f2Value) {
					merged = true
					f1Value.Set(f2Value)
				}
				continue
			}
			var fieldMerged bool
			fieldMerged, err = n.mergeIntoInherited(f1Value, f2Value, setInherited)
			if err != nil {
				return
			}
			if setInherited && hasInherited && inheritedField.Type.Kind() == reflect.Bool {
				base.FieldByIndex(inheritedField.Index).Set(reflect.ValueOf(!fieldMerged))
			}
			if fieldMerged {
				merged = true
			}
		}
	case reflect.Map:
		for _, k := range pool.MapKeys() {
			poolVal := pool.MapIndex(k)
			if n.AllowMapEmpty || !n.isEmpty(poolVal) {
				merged = true
				if base.IsNil() {
					base.Set(reflect.MakeMap(reflect.MapOf(k.Type(), poolVal.Type())))
				}
				base.SetMapIndex(k, poolVal)
			} else {
				base.SetMapIndex(k, reflect.Value{})
			}
		}
	case reflect.Slice:
		if n.SliceAdd {
			base.Set(reflect.AppendSlice(base, pool))
			break
		}
		fallthrough
	default:
		if !n.isEmpty(pool) {
			merged = true
			base.Set(pool)
		}
	}
	return
}

func (n *ScopedConfig) isEmpty(valValue reflect.Value) bool {
	switch valValue.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice:
		if valValue.IsNil() {
			return true
		}
	}
	if n.AllowEmpty {
		return false
	}
	if valValue.Kind() == reflect.Slice && valValue.Len() == 0 {
		return true
	}
	cmpValue := valValue
	zero := reflect.Zero(valValue.Type()).Interface()
	if valValue.Kind() == reflect.Ptr {
		if n.PtrNilIsEmpty {
			return valValue.IsNil()
		}
		cmpValue = valValue.Elem()
		zero = reflect.Zero(valValue.Elem().Type()).Interface()
	}
	return reflect.DeepEqual(cmpValue.Interface(), zero)
}

func (n *ScopedConfig) collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	coll := conn.Collection(n.coll)
	err = coll.EnsureIndex(mgo.Index{Key: []string{"name", "pool"}, Unique: true})
	if err != nil {
		coll.Close()
		return nil, err
	}
	return coll, nil
}
