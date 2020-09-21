package test

import (
	"encoding/json"

	"gopkg.in/check.v1"
)

var JSONEquals check.Checker = &jsonEquals{}

type jsonEquals struct{}

func (*jsonEquals) Info() *check.CheckerInfo {
	return &check.CheckerInfo{Name: "JSONEquals", Params: []string{"obtained", "expected"}}

}

func (*jsonEquals) Check(params []interface{}, names []string) (result bool, error string) {
	data0, err := json.Marshal(params[0])
	if err != err {
		return false, err.Error()
	}

	data1, err := json.Marshal(params[1])
	if err != err {
		return false, err.Error()
	}

	if string(data0) == string(data1) {
		return true, ""
	}

	return check.DeepEquals.Check(params, names)
}
