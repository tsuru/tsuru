// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package user

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/db"
	"github.com/tsuru/gandalf/fs"
	tsurufs "github.com/tsuru/tsuru/fs"
	"golang.org/x/crypto/ssh"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrDuplicateKey = errors.New("Duplicate key")
	ErrInvalidKey   = errors.New("Invalid key")
	ErrKeyNotFound  = errors.New("Key not found")
)

type Key struct {
	Name      string
	Body      string
	Comment   string
	UserName  string
	CreatedAt time.Time
}

func newKey(name, user, raw string) (*Key, error) {
	key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return nil, ErrInvalidKey
	}
	body := ssh.MarshalAuthorizedKey(key.(ssh.PublicKey))
	k := Key{
		Name:      name,
		Body:      string(body),
		Comment:   comment,
		UserName:  user,
		CreatedAt: time.Now(),
	}
	return &k, nil
}

func (k *Key) String() string {
	parts := make([]string, 1, 2)
	parts[0] = strings.TrimSpace(k.Body)
	if k.Comment != "" {
		parts = append(parts, k.Comment)
	}
	return strings.Join(parts, " ")
}

func (k *Key) format() string {
	binPath, err := config.GetString("bin-path")
	if err != nil {
		panic(err)
	}
	keyFmt := `no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty,command="%s %s" %s` + "\n"
	return fmt.Sprintf(keyFmt, binPath, k.UserName, k)
}

func (k *Key) dump(w io.Writer) error {
	formatted := k.format()
	n, err := fmt.Fprint(w, formatted)
	if err != nil {
		return err
	}
	if n != len(formatted) {
		return io.ErrShortWrite
	}
	return nil
}

func authKey() string {
	if path, _ := config.GetString("authorized-keys-path"); path != "" {
		return path
	}
	var home string
	if current, err := user.Current(); err == nil {
		home = current.HomeDir
	} else {
		home = os.ExpandEnv("$HOME")
	}
	return path.Join(home, ".ssh", "authorized_keys")
}

// creates a copy of the authorized_keys and returns it, with the file cursor
// pointing at the first byte of the file.
func copyFile() (tsurufs.File, error) {
	path := authKey()
	fi, statErr := fs.Filesystem().Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	dstPath := path + ".tmp"
	dst, err := fs.Filesystem().OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	if !os.IsNotExist(statErr) {
		original, err := fs.Filesystem().Open(path)
		if err != nil {
			return nil, err
		}
		defer original.Close()
		n, err := io.Copy(dst, original)
		if err != nil {
			dst.Close()
			return nil, err
		}
		if n != fi.Size() {
			dst.Close()
			return nil, io.ErrShortWrite
		}
		dst.Seek(0, 0)
	}
	return dst, nil
}

// moveFile writes the authorized key file atomically.
func moveFile(fromPath string) error {
	return fs.Filesystem().Rename(fromPath, authKey())
}

func writeKey(k *Key) error {
	file, err := copyFile()
	if err != nil {
		return err
	}
	defer file.Close()
	file.Seek(0, 2)
	err = k.dump(file)
	if err != nil {
		return err
	}
	return moveFile(file.Name())
}

func addKey(name, body, username string) error {
	key, err := newKey(name, username, body)
	if err != nil {
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Key().Insert(key)
	if err != nil {
		if mgo.IsDup(err) {
			return ErrDuplicateKey
		}
		return err
	}
	return writeKey(key)
}

func updateKey(name, body, username string) error {
	newK, err := newKey(name, username, body)
	if err != nil {
		return err
	}
	var oldK Key
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Key().Find(bson.M{"name": name, "username": username}).One(&oldK)
	if err != nil {
		return ErrKeyNotFound
	}
	err = remove(&oldK)
	if err != nil {
		return err
	}
	err = writeKey(newK)
	if err != nil {
		writeKey(&oldK)
		return err
	}
	return conn.Key().Update(bson.M{"name": name, "username": username}, newK)
}

func addKeys(keys map[string]string, username string) error {
	for name, k := range keys {
		err := addKey(name, k, username)
		if err != nil {
			return err
		}
	}
	return nil
}

func remove(k *Key) error {
	formatted := k.format()
	file, err := copyFile()
	if err != nil {
		return err
	}
	defer file.Close()
	lines := make([]string, 0, 10)
	reader := bufio.NewReader(file)
	line, _ := reader.ReadString('\n')
	for line != "" {
		if line != formatted {
			lines = append(lines, line)
		}
		line, _ = reader.ReadString('\n')
	}
	file.Truncate(0)
	file.Seek(0, 0)
	content := strings.Join(lines, "")
	n, err := file.WriteString(content)
	if err != nil {
		return err
	}
	if n != len(content) {
		return io.ErrShortWrite
	}
	return moveFile(file.Name())
}

func removeUserKeys(username string) error {
	var keys []Key
	q := bson.M{"username": username}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Key().Find(q).All(&keys)
	if err != nil {
		return err
	}
	conn.Key().RemoveAll(q)
	for _, k := range keys {
		remove(&k)
	}
	return nil
}

// removes a key from the database and the authorized_keys file.
func removeKey(name, username string) error {
	var k Key
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Key().Find(bson.M{"name": name, "username": username}).One(&k)
	if err != nil {
		return ErrKeyNotFound
	}
	conn.Key().Remove(k)
	return remove(&k)
}

type KeyList []Key

func (keys KeyList) MarshalJSON() ([]byte, error) {
	m := make(map[string]string, len(keys))
	for _, key := range keys {
		m[key.Name] = key.String()
	}
	return json.Marshal(m)
}

// ListKeys lists all user's keys.
//
// If the user is not found, returns an error
func ListKeys(uName string) (KeyList, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	n, err := conn.User().FindId(uName).Count()
	if err != nil || n != 1 {
		return nil, ErrUserNotFound
	}
	var keys []Key
	err = conn.Key().Find(bson.M{"username": uName}).All(&keys)
	return KeyList(keys), err
}
