package gitosis

import (
	"fmt"
	"os"
	"path"
	"syscall"
)

// buildAndStoreKeyFile adds a key to key dir, returning the name
// of the file containing the new public key. This name should
// be stored for future remotion of the key.
//
// It is up to the caller to add the keyfile name to the gitosis
// configuration file. One possible use to this function is together
// with addMember function:
//
//     keyfile, _ := buildAndStoreKeyFile("opeth", "face-of-melinda")
//     addMember("bands", keyfile) // adds keyfile to group bands
//     addMember("sweden", keyfile) // adds keyfile to group sweden
func buildAndStoreKeyFile(member, key string) (string, error) {
	p, err := getKeydirPath()
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(p, 0755)
	if err != nil {
		return "", err
	}
	filename, err := nextAvailableKey(p, member)
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
	commitMsg := fmt.Sprintf("Added %s keyfile.", filename)
	err = pushToGitosis(commitMsg)
	if err != nil {
		return "", err
	}
	return filename, nil
}

// deleteKeyFile deletes the keyfile in the keydir
//
// After deleting the keyfile, the user will not be able
// to push to the repository, even if the keyfile name still
// is in the gitosis configuration file.
func deleteKeyFile(keyfilename string) error {
	p, err := getKeydirPath()
	if err != nil {
		return err
	}
	keypath := path.Join(p, keyfilename)
	err = os.Remove(keypath)
	if err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("Deleted %s keyfile.", keyfilename)
	return pushToGitosis(commitMsg)
}

func nextAvailableKey(keydirname, member string) (string, error) {
	keydir, err := os.Open(keydirname)
	if err != nil {
		return "", err
	}
	defer keydir.Close()
	filenames, err := keydir.Readdirnames(0)
	if err != nil {
		return "", err
	}
	pattern := member + "_key%d.pub"
	counter := 1
	filename := fmt.Sprintf(pattern, counter)
	for _, f := range filenames {
		if f == filename {
			counter++
			filename = fmt.Sprintf(pattern, counter)
		}
	}
	return filename, nil
}
