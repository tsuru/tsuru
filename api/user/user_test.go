package api

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
	}

	c.Assert(email, Equals, u.Email)
	c.Assert(password, Equals, u.Password)
}
