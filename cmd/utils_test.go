package cmd

import . "launchpad.net/gocheck"

func (s *S) TestWriteToken(c *C) {
	err := WriteToken("abc")
	c.Assert(err, IsNil)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "abc")
}

func (s *S) TestReadToken(c *C) {
	err := WriteToken("123")
	c.Assert(err, IsNil)
	token, err := ReadToken()
	c.Assert(err, IsNil)
	c.Assert(token, Equals, "123")
}

func (s *S) TestShowServicesInstancesList(c *C) {
	expected := `+----------+-----------+
| Services | Instances |
+----------+-----------+
| mongodb  | my_nosql  |
+----------+-----------+
`
	b := `[{"service": "mongodb", "instances": ["my_nosql"]}]`
	result, err := ShowServicesInstancesList([]byte(b))
	c.Assert(err, IsNil)
	c.Assert(string(result), Equals, expected)
}
