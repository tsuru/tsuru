package testing

type hasIndexChecker struct{}

func (c *hasIndexChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasIndexChecker", Params: []string{"collection", "key"}}
}

func (c *hasIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*Collection)
	if !ok {
		return false, "first parameter should be a Collection"
	}
	key, ok := params[1].([]string)
	if !ok {
		return false, "second parameter should be the key, as used for mgo index declaration (slice of strings)"
	}
	indexes, err := collection.Indexes()
	if err != nil {
		return false, "failed to get collection indexes: " + err.Error()
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return true, ""
		}
	}
	return false, ""
}

var HasIndex gocheck.Checker = &hasIndexChecker{}

type hasUniqueIndexChecker struct{}

func (c *hasUniqueIndexChecker) Info() *gocheck.CheckerInfo {
	return &gocheck.CheckerInfo{Name: "HasUniqueField", Params: []string{"collection", "key"}}
}

func (c *hasUniqueIndexChecker) Check(params []interface{}, names []string) (bool, string) {
	collection, ok := params[0].(*Collection)
	if !ok {
		return false, "first parameter should be a Collection"
	}
	key, ok := params[1].([]string)
	if !ok {
		return false, "second parameter should be the key, as used for mgo index declaration (slice of strings)"
	}
	indexes, err := collection.Indexes()
	if err != nil {
		return false, "failed to get collection indexes: " + err.Error()
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return index.Unique, ""
		}
	}
	return false, ""
}

var HasUniqueIndex gocheck.Checker = &hasUniqueIndexChecker{}
