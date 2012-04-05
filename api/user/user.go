package user

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha512"
	"errors"
	"fmt"
	"launchpad.net/mgo/bson"
	"github.com/timeredbull/tsuru/database"
	"time"
)

const (
	SALT        = "tsuru-salt"
	TOKENEXPIRE = 7 * 24 * time.Hour
	TOKENKEY    = "tsuru-key"
)

func hashPassword(password string) string {
	salt := []byte(SALT)
	return fmt.Sprintf("%x", pbkdf2.Key([]byte(password), salt, 4096, len(salt)*8, sha512.New))
}

type User struct {
	Id       bson.ObjectId "_id"
	Email    string
	Password string
	Tokens   []Token
}

type Token struct {
	Token      []byte
	ValidUntil time.Time
}

func (u *User) Create() error {
	u.hashPassword()
	u.Id = bson.NewObjectId()

	c := database.Mdb.C("users")
	err := c.Insert(u)

	return err
}

func (u *User) hashPassword() {
	u.Password = hashPassword(u.Password)
}

func (u *User) Get() error {
	var filter = bson.M{}

	if u.Id.Valid() {
		filter["_id"] = u.Id
		// field = "id"
		// args[0] = u.Id
	} else {
		filter["email"] = u.Email
		// field = "email"
		// args[0] = u.Email
	}

	var result User
	c := database.Mdb.C("users")
	err := c.Find(filter).One(&result)
	// row := database.Db.QueryRow(fmt.Sprintf("SELECT id, email, password FROM users WHERE %s = ?", field), args...)
	// err := row.Scan(&u.Id, &u.Email, &u.Password)

	return err
}

func (u *User) Login(password string) bool {
	hashedPassword := hashPassword(password)
	return u.Password == hashedPassword
}

func NewToken(u *User) (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("Impossible to generate tokens for users without email")
	}
	h := sha512.New()
	h.Write([]byte(u.Email))
	h.Write([]byte(TOKENKEY))
	h.Write([]byte(time.Now().Format(time.UnixDate)))
	t := Token{}
	t.ValidUntil = time.Now().Add(TOKENEXPIRE)
	t.Token = h.Sum(nil)
	return &t, nil
}

func (u *User) CreateToken() (*Token, error) {
	if !u.Id.Valid() {
		return nil, errors.New("User does not have an id")
	}

	t, _ := NewToken(u)
	u.Tokens = append(u.Tokens, *t)

	selector := map[string]interface{}{ "_id": u.Id }
	change := map[string]interface{}{ "tokens": u.Tokens } // should ensure that it maintains any old tokens

	c := database.Mdb.C("users")
	err := c.Update(selector, change)

	return t, err
	// _, err := database.Db.Exec("INSERT INTO usertokens (user_id, token, valid_until) VALUES (?, ?, ?)", t.U.Id, fmt.Sprintf("%x", t.Token), t.ValidUntil.Format(time.UnixDate))
}

func GetUserByToken(token string) (*User, error) {
	var valid string
	u := new(User)
	query := map[string]interface{}{ "tokens.token": token }
	c := database.Mdb.C("users")
	err := c.Find(query).One(&u)
	// row := database.Db.QueryRow("SELECT u.id, u.email, u.password, t.valid_until FROM users u INNER JOIN usertokens t ON t.user_id = u.id WHERE t.token = ?", token)
	// err := row.Scan(&u.Id, &u.Email, &u.Password, &valid)
	// if err != nil {
	// 	return nil, errors.New("Token not found")
	// }
	fmt.Println("oooi", u.Tokens)
	t, err := time.Parse(time.UnixDate, valid)
	if err != nil {
		return nil, err
	}
	if t.Sub(time.Now()) < 1 {
		return nil, errors.New("Token has expired")
	}
	return u, nil
}
