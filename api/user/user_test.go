package user

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"fmt"
	. "github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo/bson"
	"time"
)

func (s *S) TestCreateUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var result User
	query := map[string]interface{}{ "_id": u.Id }
	collection := Mdb.C("users")
	err = collection.Find(query).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Email, Equals, u.Email)
	// var email, password string
	// rows, err := s.db.Query("SELECT email, password FROM users WHERE id = (SELECT max(id) FROM users)")
	// c.Assert(err, IsNil)
	// if rows.Next() {
	// 	rows.Scan(&email, &password)
	// 	rows.Close()
	// }
	// c.Assert(email, Equals, u.Email)
}

func (s *S) TestCreateUserHashesThePasswordUsingPBKDF2SHA512AndSalt(c *C) {
	salt := []byte(SALT)
	expectedPassword := fmt.Sprintf("%x", pbkdf2.Key([]byte("123456"), salt, 4096, len(salt)*8, sha512.New))
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var result User
	query := map[string]interface{}{ "_id": u.Id }
	collection := Mdb.C("users")
	err = collection.Find(query).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Password, Equals, expectedPassword)

	// var password string
	// row := s.db.QueryRow("SELECT password FROM users WHERE id = (SELECT max(id) FROM users)")
	// row.Scan(&password)
	// c.Assert(password, Equals, expectedPassword)
}

func (s *S) TestCreateUserReturnsErrorWhenAnyErrorHappens(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)

	err = u.Create()
	c.Assert(err, NotNil)
}

func (s *S) TestGetUserById(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	var result User
	query := map[string]interface{}{ "_id": u.Id }
	collection := Mdb.C("users")
	err = collection.Find(query).One(&result)
	c.Assert(err, IsNil)
	c.Assert(u.Email, Equals, result.Email)
}

func (s *S) TestGetUserByEmail(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, IsNil)

	u = User{Email: "wolverine@xmen.com"}
	err = u.Get()
	c.Assert(err, IsNil)
	c.Assert(u.Id.Valid(), Equals, true)
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
}

func (s *S) TestGetUserReturnsErrorWhenNoUserIsFound(c *C) {
	u := User{Id: bson.NewObjectId()}
	err := u.Get()
	c.Assert(err, NotNil)
}

func (s *S) TestUserLoginReturnsTrueIfThePasswordMatches(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	u.hashPassword()
	c.Assert(u.Login("123"), Equals, true)
}

func (s *S) TestUserLoginReturnsFalseIfThePasswordDoesNotMatch(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	u.hashPassword()
	c.Assert(u.Login("1234"), Equals, false)
}

func (s *S) TestNewTokenIsStoredInUser(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	u.Create()

	t, err := u.CreateToken()
	c.Assert(err, IsNil)

	c.Assert(u.Email, Equals, "wolverine@xmen.com")
	c.Assert(string(u.Tokens[0].Token), Equals, string(t.Token))
}

func (s *S) TestNewTokenReturnsErroWhenUserReferenceDoesNotContainsEmail(c *C) {
	u := User{}
	t, err := NewToken(&u)
	c.Assert(t, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Impossible to generate tokens for users without email$")
}

// func (s *S) TestNewTokenReturnsErrorWhenUserIsNil(c *C) {
// 	t, err := NewToken(nil)
// 	c.Assert(t, IsNil)
// 	c.Assert(err, NotNil)
// 	c.Assert(err, ErrorMatches, "^User is nil$")
// }

func (s *S) TestCreateTokenShouldSaveTheTokenInUserInTheDatabase(c *C) {
	var t *Token
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	_, err = u.CreateToken()
	c.Assert(err, IsNil)

	var result User
	collection := Mdb.C("users")
	err = collection.Find(nil).One(&result)
	c.Assert(err, IsNil)
	c.Assert(result.Tokens[0].Token, Equals, t.Token)
	c.Assert(result.Id, Equals, u.Id)
}

func (s *S) TestCreateTokenShouldReturnErrorIfTheProvidedUserDoesNotHaveId(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	_, err := u.CreateToken()
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User does not have an id$")
}

func (s *S) TestGetUserByToken(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	user, err := GetUserByToken(fmt.Sprintf("%x", t.Token))
	c.Assert(err, IsNil)
	c.Assert(user, DeepEquals, &u)
}

func (s *S) TestGetUserByTokenShouldReturnErrorWhenTheGivenTokenDoesNotExist(c *C) {
	user, err := GetUserByToken("i don't exist")
	c.Assert(user, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Token not found$")
}

func (s *S) TestGetUserByTokenShouldReturnErrorWhenTheGivenTokenHasExpired(c *C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	t, err := u.CreateToken()
	c.Assert(err, IsNil)
	/* s.db.Exec("UPDATE usertokens SET valid_until = ?", time.Now().Add(-24*time.Hour).Format(time.UnixDate)) */
	collection := Mdb.C("users")
	change := map[string]map[string]interface{}{ "token":{ "valid_until": time.Now().Add(-24*time.Hour).Format(time.UnixDate) } }
	err = collection.Update(nil, change)
	user, err := GetUserByToken(fmt.Sprintf("%x", t.Token))
	c.Assert(user, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Token has expired$")
}
