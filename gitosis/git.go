package gitosis

import (
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"os/exec"
	"path"
)

// represents a git repository
type repository struct {
	path string
}

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
	out, err := runGit("add", ".")
	if err != nil {
		log.Print(out)
		return err
	}
	out, err = runGit("commit", "-am", cMsg)
	if err != nil {
		log.Print(out)
		return err
	}
	out, err = runGit("push", "origin", "master")
	if err != nil {
		log.Print(out)
	}
	return err
}

func writeCommitPush(c *ini.Config, commit string) error {
	m, err := newGitosisManager()
	if err != nil {
		return err
	}
	err = c.WriteFile(m.confPath, 0744, "gitosis configuration file")
	if err != nil {
		return err
	}
	err = pushToGitosis(commit)
	if err != nil {
		return err
	}
	return nil
}
