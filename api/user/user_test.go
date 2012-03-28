package user

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/timeredbull/tsuru/database"
	. "launchpad.net/gocheck"
	"os"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	db *sql.DB
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.db, err = sql.Open("sqlite3", "./data.db")
	if err != nil {
		panic(err)
	}
	database.Db = s.db
	_, err = s.db.Exec("create table users (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, email VARCHAR(100) UNIQUE, password VARCHAR(100))")
	c.Assert(err, IsNil)
}

func (s *S) TearDownSuite(c *C) {
	s.db.Close()
	os.Remove("./data.db")
}

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
	c.Assert(password, Equals, u.Password)
	_, err = s.db.Exec(`DELETE FROM users WHERE email="wolverine@xmen.com"`)
	c.Assert(err, IsNil)
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
	c.Assert(u.Password, Equals, "123456")
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
	c.Assert(u.Email, Equals, "wolverine@xmen.com")
	c.Assert(u.Password, Equals, "123456")
	_, err = s.db.Exec(`DELETE FROM users WHERE email="wolverine@xmen.com"`)
	c.Assert(err, IsNil)
}

func (s *S) TestGetUserReturnsErrorWhenNoUserIsFound(c *C) {
	u := User{Id: 10}
	err := u.Get()
	c.Assert(err, NotNil)
}
