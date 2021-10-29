package app

import (
	"testing"

	"gopkg.in/check.v1"
)

var _ = check.Suite(&S{})

type S struct {
}

func (s S) TestMetadataUpdateList(c *check.C) {
	result := updateList([]MetadataItem{{Name: "found-item", Value: "value"}}, []MetadataItem{{Name: "not-found-item", Delete: true}})
	c.Assert(result, check.DeepEquals, []MetadataItem{{Name: "found-item", Value: "value"}})

	result = updateList([]MetadataItem{{Name: "found-item", Value: "value"}}, []MetadataItem{{Name: "found-item", Value: "new-value"}})
	c.Assert(result, check.DeepEquals, []MetadataItem{{Name: "found-item", Value: "new-value"}})

	result = updateList([]MetadataItem{}, []MetadataItem{{Name: "found-item", Value: "new-value"}})
	c.Assert(result, check.DeepEquals, []MetadataItem{{Name: "found-item", Value: "new-value"}})
}

func Test(t *testing.T) {
	check.TestingT(t)
}
