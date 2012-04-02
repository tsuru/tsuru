package user

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"fmt"
	. "launchpad.net/gocheck"
)

func (s *S) TestCreateUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var email, password string
	rows, err := s.db.Query("SELECT email, password FROM users WHERE id = (SELECT max(id) FROM users)")
	c.Assert(err, IsNil)
	if rows.Next() {
		rows.Scan(&email, &password)
		rows.Close()
	}
	c.Assert(email, Equals, u.Email)
}

func (s *S) TestCreateUserHashesThePasswordUsingPBKDF2SHA512AndSalt(c *C) {
	salt := []byte(SALT)
	expectedPassword := fmt.Sprintf("%x", pbkdf2.Key([]byte("123456"), salt, 4096, len(salt)*8, sha512.New))
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var password string
	row := s.db.QueryRow("SELECT password FROM users WHERE id = (SELECT max(id) FROM users)")
	row.Scan(&password)
	c.Assert(password, Equals, expectedPassword)
}

func (s *S) TestCreateUserReturnsErrorWhenAnyHappen(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)

	err = u.Create()
	c.Assert(err, NotNil)
	_, err = s.db.Exec(`DELETE FROM users WHERE email="wolverine@xmen.com"`)
	c.Assert(err, IsNil)
}

func (s *S) TestGetUserById(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var id int
	rows, err := s.db.Query("SELECT max(id) FROM users")
	c.Assert(err, IsNil)
	if rows.Next() {
		rows.Scan(&id)
		rows.Close()
	}
	u = User{Id: id}
	err = u.Get()
	c.Assert(err, IsNil)
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
	_, err = s.db.Exec(`DELETE FROM users WHERE email="wolverine@xmen.com"`)
	c.Assert(err, IsNil)
}

func (s *S) TestGetUserByEmail(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	u = User{Email: "wolverine@xmen.com"}
	err = u.Get()
	c.Assert(err, IsNil)
	c.Assert(u.Id > 0, Equals, true)
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
	_, err = s.db.Exec(`DELETE FROM users WHERE email="wolverine@xmen.com"`)
	c.Assert(err, IsNil)
}

func (s *S) TestGetUserReturnsErrorWhenNoUserIsFound(c *C) {
	u := User{Id: 10}
	err := u.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestNewTokenStoresUserReference(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	t, err := NewToken(&u)
	c.Assert(err, IsNil)
	c.Assert(t.U.Email, Equals, "wolverine@xmen.com")
}

func (s *S) TestNewTokenReturnsErroWhenUserReferenceDoesNotContainsEmail(c *C) {
	u := User{}
	t, err := NewToken(&u)
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Impossible to generate tokens for users without email$")
}

func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *C) {
	t, err := NewToken(nil)
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User is nil$")
}

func (s *S) TestCreateTokenShouldSaveTheTokenInTheDatabase(c *C) {
	var t *Token
	var userId, tokenId int
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	t, err = NewToken(&u)
	c.Assert(err, IsNil)
	err = t.Create()
	c.Assert(err, IsNil)
	row := s.db.QueryRow("SELECT id, user_id FROM usertokens ORDER BY id DESC LIMIT 1")
	row.Scan(&tokenId, &userId)
	c.Assert(tokenId > 0, Equals, true)
	c.Assert(userId, Equals, u.Id)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveId(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	t, err := NewToken(&u)
	c.Assert(err, IsNil)
	err = t.Create()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User does not have an id$")
}
