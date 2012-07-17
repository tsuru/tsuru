package auth

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	"sync"
)

type Team struct {
	Name  string
	Users []User
}

func (t *Team) containsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user.Email {
			return true
		}
	}
	return false
}

func (t *Team) addUser(u *User) error {
	if t.containsUser(u) {
		return errors.New(fmt.Sprintf("User %s is alread in the team %s.", u.Email, t.Name))
	}
	t.Users = append(t.Users, *u)
	return nil
}

func (t *Team) removeUser(u *User) error {
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

func GetTeamsNames(teams []Team) []string {
	tn := make([]string, len(teams))
	for i, t := range teams {
		tn[i] = t.Name
	}
	return tn
}

func CheckUserAccess(teamNames []string, u *User) bool {
	q := bson.M{"name": bson.M{"$in": teamNames}}
	var teams []Team
	db.Session.Teams().Find(q).All(&teams)
	var wg sync.WaitGroup
	found := make(chan bool)
	for _, team := range teams {
		wg.Add(1)
		go func(t Team) {
			if t.containsUser(u) {
				found <- true
			}
			wg.Done()
		}(team)
	}
	go func() {
		wg.Wait()
		found <- false
	}()
	return <-found
}
