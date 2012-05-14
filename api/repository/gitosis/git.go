package gitosis

import (
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"os/exec"
	"path"
	"sync"
)

var mut sync.Mutex

func getKeydirPath() (string, error) {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Print(err)
		return "", err
	}
	return path.Join(repoPath, "keydir"), nil
}

func runGit(args ...string) (string, error) {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		return "", err
	}
	workTree := "--work-tree=" + repoPath
	gitDir := "--git-dir=" + path.Join(repoPath, ".git")
	gitArgs := []string{workTree, gitDir}
	gitArgs = append(gitArgs, args...)
	output, err := exec.Command("git", gitArgs...).CombinedOutput()
	return string(output), err
}

// Add, commit and push all changes in gitosis repository to it's
// bare.
func pushToGitosis(cMsg string) error {
	Lock()
	defer Unlock()
	_, err := runGit("add", ".")
	if err != nil {
		return err
	}
	_, err = runGit("commit", "-am", cMsg)
	if err != nil {
		return err
	}
	_, err = runGit("push", "origin", "master")
	return err
}

func writeCommitPush(c *ini.Config, commit string) error {
	confPath, err := ConfPath()
	if err != nil {
		return err
	}
	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	if err != nil {
		return err
	}
	err = pushToGitosis(commit)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func Lock() {
	mut.Lock()
}

func Unlock() {
	mut.Unlock()
}
