package repository

import (
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

func (r *repository) commit(message string) error {
	_, err := r.run("add", ".")
	if err != nil {
		return err
	}
	_, err = r.run("commit", "-am", message)
	return err
}

func (r *repository) push(remote, branch string) error {
	_, err := r.run("push", remote, branch)
	return err
}

func (r *repository) getPath(p ...string) string {
	args := []string{r.path}
	args = append(args, p...)
	return path.Join(args...)
}
