package docker

import check "gopkg.in/check.v1"

func (s *S) TestBuilderCollection(c *check.C) {
	collection := s.b.Collection()
	defer collection.Close()
	c.Assert(collection.Name, check.Equals, "dockerbuilder")
}
