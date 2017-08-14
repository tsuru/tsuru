package pool

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tsuru/tsuru/db"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type PoolConstraint struct {
	PoolExpr  string
	Field     string
	Values    []string
	Blacklist bool
}

func (c *PoolConstraint) checkExact(v string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.Values {
		if r == v {
			return !c.Blacklist
		}
	}
	return c.Blacklist
}

func (c *PoolConstraint) check(v string) bool {
	if c == nil {
		return false
	}
	for _, r := range c.Values {
		pattern := exprAsGlobPattern(r)
		if match, _ := regexp.MatchString(pattern, v); match {
			return !c.Blacklist
		}
	}
	return c.Blacklist
}

func (c *PoolConstraint) AllowsAll() bool {
	if c == nil || c.Blacklist {
		return false
	}
	return c.checkExact("*")
}

type constraintList []*PoolConstraint

func (l constraintList) Len() int      { return len(l) }
func (l constraintList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l constraintList) Less(i, j int) bool {
	lenI, lenJ := len(l[i].PoolExpr), len(l[j].PoolExpr)
	if lenI == lenJ {
		return strings.Count(l[i].PoolExpr, "*") < strings.Count(l[j].PoolExpr, "*")
	}
	return lenI > lenJ
}

func SetPoolConstraint(c *PoolConstraint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	isValid := validateConstraintType(c.Field)
	if !isValid {
		return ErrInvalidConstraintType
	}
	if len(c.Values) == 0 || (len(c.Values) == 1 && c.Values[0] == "") {
		errRem := conn.PoolsConstraints().Remove(bson.M{"poolexpr": c.PoolExpr, "field": c.Field})
		if errRem != mgo.ErrNotFound {
			return errRem
		}
		return nil
	}
	_, err = conn.PoolsConstraints().Upsert(bson.M{"poolexpr": c.PoolExpr, "field": c.Field}, c)
	return err
}

func AppendPoolConstraint(c *PoolConstraint) error {
	return appendPoolConstraint(c.PoolExpr, c.Field, c.Values...)
}

func validateConstraintType(c string) bool {
	for _, v := range validConstraintTypes {
		if c == v {
			return true
		}
	}
	return false
}

func appendPoolConstraint(poolExpr string, field string, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.PoolsConstraints().Upsert(
		bson.M{"poolexpr": poolExpr, "field": field},
		bson.M{"$addToSet": bson.M{"values": bson.M{"$each": values}}},
	)
	return err
}

func removePoolConstraint(poolExpr string, field string, values ...string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.PoolsConstraints().Update(bson.M{"poolexpr": poolExpr, "field": field}, bson.M{"$pullAll": bson.M{"values": values}})
}

func getPoolsSatisfyConstraints(exactCheck bool, field string, values ...string) ([]Pool, error) {
	pools, err := listPools(nil)
	if err != nil {
		return nil, err
	}
	var satisfying []Pool
loop:
	for _, p := range pools {
		constraints, err := getConstraintsForPool(p.Name, field)
		if err != nil {
			return nil, err
		}
		c, ok := constraints[field]
		if !ok || c.PoolExpr != p.Name {
			continue
		}
		for _, v := range values {
			if exactCheck && !c.checkExact(v) {
				continue loop
			}
			if !exactCheck && !c.check(v) {
				continue loop
			}
		}
		satisfying = append(satisfying, p)
	}
	return satisfying, nil
}

func getConstraintsForPool(pool string, fields ...string) (map[string]*PoolConstraint, error) {
	var query bson.M
	if len(fields) > 0 {
		query = bson.M{"field": bson.M{"$in": fields}}
	}
	constraints, err := ListPoolsConstraints(query)
	if err != nil {
		return nil, err
	}
	var matches []*PoolConstraint
	for _, c := range constraints {
		pattern := exprAsGlobPattern(c.PoolExpr)
		match, err := regexp.MatchString(pattern, pool)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, c)
		}
	}
	sort.Sort(constraintList(matches))
	merged := make(map[string]*PoolConstraint)
	for i := range matches {
		if _, ok := merged[matches[i].Field]; !ok {
			merged[matches[i].Field] = matches[i]
		}
	}
	return merged, nil
}

func getExactConstraintForPool(pool, field string) (*PoolConstraint, error) {
	constraints, err := ListPoolsConstraints(bson.M{"poolexpr": pool, "field": field})
	if err != nil {
		return nil, err
	}
	if len(constraints) == 0 {
		return nil, nil
	}
	return constraints[0], nil
}

func ListPoolsConstraints(query bson.M) ([]*PoolConstraint, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	constraints := []*PoolConstraint{}
	err = conn.PoolsConstraints().Find(query).All(&constraints)
	if err != nil {
		return nil, err
	}
	return constraints, nil
}
