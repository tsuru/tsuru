package repository

import (
	"fmt"
	"os/exec"
	"path"
	"sync"
)

// repository represents a git repository.
type repository struct {
	bare bool
	path string
	sync.Mutex
}

// run executes a command in the git repository, and returns the output of the
// command or an error, if any happens.
func (r *repository) run(args ...string) (string, error) {
	r.Lock()
	defer r.Unlock()
	var gitDir, workTree string
	workTree = "--work-tree=" + r.path
	if r.bare {
		gitDir = "--git-dir=" + r.path
	} else {
		gitDir = "--git-dir=" + path.Join(r.path, ".git")
	}
	gitArgs := []string{workTree, gitDir}
	gitArgs = append(gitArgs, args...)
	output, err := exec.Command("git", gitArgs...).CombinedOutput()
	return string(output), err
}

// commit commits a change in the repository
//
// It commits *everything* (git add . + git commit -am).
func (r *repository) commit(message string) error {
	out, err := r.run("add", ".")
	if err != nil {
		return fmt.Errorf("Failed to add files: %s.\nOutput: %s.", err, out)
	}
	out, err = r.run("commit", "-am", message)
	if err != nil {
		return fmt.Errorf("Failed to run commit: %s.\nOutput: %s.", err, out)
	}
	return nil
}

// push pushes commits to a remote.
func (r *repository) push(remote, branch string) error {
	out, err := r.run("push", remote, branch)
	if err != nil {
		if err != nil {
			return fmt.Errorf("Failed to push: %s.\nOutput: %s.", err, out)
		}
	}
	return err
}

func (r *repository) getPath(p ...string) string {
	args := []string{r.path}
	args = append(args, p...)
	return path.Join(args...)
}
