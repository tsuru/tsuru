package auth

import (
	"errors"
	"fmt"
)

type Team struct {
	Name  string
	Users []*User
}

func (t *Team) ContainsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user.Email {
			return true
		}
	}
	return false
}

func (t *Team) AddUser(u *User) error {
	if t.ContainsUser(u) {
		return errors.New(fmt.Sprintf("User %s is alread in the team %s.", u.Email, t.Name))
	}
	t.Users = append(t.Users, u)
	return nil
}

func (t *Team) RemoveUser(u *User) error {
	index := -1
	for i, user := range t.Users {
		if u.Email == user.Email {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New(fmt.Sprintf("User %s is not in the team %s.", u.Email, t.Name))
	}
	last := len(t.Users) - 1
	t.Users[index] = t.Users[last]
	t.Users = t.Users[:last]
	return nil
}
