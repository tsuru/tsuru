package auth

import (
	"fmt"
	"github.com/timeredbull/tsuru/db"
	"labix.org/v2/mgo/bson"
	"sync"
)

type Team struct {
	Name  string `bson:"_id"`
	Users []string
}

func (t *Team) containsUser(u *User) bool {
	for _, user := range t.Users {
		if u.Email == user {
			return true
		}
	}
	return false
}

func (t *Team) addUser(u *User) error {
	if t.containsUser(u) {
		return fmt.Errorf("User %s is alread in the team %s.", u.Email, t.Name)
	}
	t.Users = append(t.Users, u.Email)
	return nil
}

func (t *Team) removeUser(u *User) error {
	index := -1
	for i, user := range t.Users {
		if u.Email == user {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("User %s is not in the team %s.", u.Email, t.Name)
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
	q := bson.M{"_id": bson.M{"$in": teamNames}}
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
