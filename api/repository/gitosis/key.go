package gitosis

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/log"
	"os"
	"path"
	"syscall"
)

// AddKeys adds a user's public key to the keydir
func AddKey(group, member, key string) (string, error) {
	c, err := getConfig()
	if err != nil {
		return "", err
	}
	if !c.HasSection("group " + group) {
		return "", errors.New("Group not found")
	}
	p, err := getKeydirPath()
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(p, 0755)
	if err != nil {
		return "", err
	}
	filename, actualMember, err := nextAvailableKey(p, member)
	if err != nil {
		return "", err
	}
	keyfilename := path.Join(p, filename)
	keyfile, err := os.OpenFile(keyfilename, syscall.O_WRONLY|syscall.O_CREAT, 0644)
	if err != nil {
		return "", err
	}
	defer keyfile.Close()
	n, err := keyfile.WriteString(key)
	if err != nil || n != len(key) {
		return "", err
	}
	err = addMember(group, actualMember)
	if err != nil {
		err = os.Remove(keyfilename)
		if err != nil {
			log.Panicf("Fatal error: the key file %s was left in the keydir", keyfilename)
			return "", err
		}
		return "", errors.New("Failed to add member to the group, the key file was not saved")
	}
	return filename, nil
}

func nextAvailableKey(keydirname, member string) (string, string, error) {
	keydir, err := os.Open(keydirname)
	if err != nil {
		return "", "", err
	}
	defer keydir.Close()
	filenames, err := keydir.Readdirnames(0)
	if err != nil {
		return "", "", err
	}
	pattern := member + "_key%d"
	counter := 1
	actualMember := fmt.Sprintf(pattern, counter)
	filename := actualMember + ".pub"
	for _, f := range filenames {
		if f == filename {
			counter++
			actualMember = fmt.Sprintf(pattern, counter)
			filename = actualMember + ".pub"
		}
	}
	return filename, actualMember, nil
}
