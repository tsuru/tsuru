// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/db"
	"github.com/tsuru/gandalf/fs"
	"github.com/tsuru/gandalf/multipartzip"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var tempDir string

var (
	ErrRepositoryAlreadyExists = errors.New("repository already exists")
	ErrRepositoryNotFound      = errors.New("repository not found")
)

func tempDirLocation() string {
	if tempDir == "" {
		tempDir, _ = config.GetString("repository:tempDir")
	}
	return tempDir
}

// Repository represents a Git repository. A Git repository is a record in the
// database and a directory in the filesystem (the bare repository).
type Repository struct {
	Name          string `bson:"_id"`
	Users         []string
	ReadOnlyUsers []string
	IsPublic      bool
}

type Links struct {
	TarArchive string `json:"tarArchive"`
	ZipArchive string `json:"zipArchive"`
}

type GitUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

func (gu GitUser) String() string {
	return fmt.Sprintf("%s <%s>", gu.Name, gu.Email)
}

type GitCommit struct {
	Message   string
	Author    GitUser
	Committer GitUser
	Branch    string
}

type Ref struct {
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Author    *GitUser `json:"author"`
	Committer *GitUser `json:"committer"`
	Tagger    *GitUser `json:"tagger"`
	Links     *Links   `json:"_links"`
	Subject   string   `json:"subject"`
	CreatedAt string   `json:"createdAt"`
}

type GitLog struct {
	Ref       string   `json:"ref"`
	Author    *GitUser `json:"author"`
	Committer *GitUser `json:"committer"`
	Subject   string   `json:"subject"`
	CreatedAt string   `json:"createdAt"`
	Parent    []string `json:"parent"`
}

type GitHistory struct {
	Commits []GitLog `json:"commits"`
	Next    string   `json:"next"`
}

// exists returns whether the given file or directory exists or not
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// MarshalJSON marshals the Repository in json format.
func (r *Repository) MarshalJSON() ([]byte, error) {
	data := map[string]interface{}{
		"name":    r.Name,
		"public":  r.IsPublic,
		"ssh_url": r.ReadWriteURL(),
		"git_url": r.ReadOnlyURL(),
	}
	return json.Marshal(&data)
}

// New creates a representation of a git repository. It creates a Git
// repository using the "bare-dir" setting and saves repository's meta data in
// the database.
func New(name string, users, readOnlyUsers []string, isPublic bool) (*Repository, error) {
	log.Debugf("Creating repository %q", name)
	r := &Repository{Name: name, Users: users, ReadOnlyUsers: readOnlyUsers, IsPublic: isPublic}
	if v, err := r.isValid(); !v {
		log.Errorf("repository.New: Invalid repository %q: %s", name, err)
		return r, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Repository().Insert(r)
	if err != nil {
		if mgo.IsDup(err) {
			return nil, ErrRepositoryAlreadyExists
		}
		return nil, err
	}
	if err := newBare(name); err != nil {
		log.Errorf("repository.New: Error creating bare repository for %q: %s", name, err)
		conn.Repository().Remove(bson.M{"_id": r.Name})
		return r, err
	}
	barePath := barePath(name)
	if barePath != "" && isPublic {
		if f, createErr := fs.Filesystem().Create(barePath + "/git-daemon-export-ok"); createErr == nil {
			f.Close()
		}
	}
	return r, err
}

// Get find a repository by name.
func Get(name string) (Repository, error) {
	var r Repository
	conn, err := db.Conn()
	if err != nil {
		return r, err
	}
	defer conn.Close()
	err = conn.Repository().FindId(name).One(&r)
	if err == mgo.ErrNotFound {
		return r, ErrRepositoryNotFound
	}
	return r, err
}

// Remove deletes the repository from the database and removes it's bare Git
// repository.
func Remove(name string) error {
	log.Debugf("Removing repository %q", name)
	if err := removeBare(name); err != nil {
		log.Errorf("repository.Remove: Error removing bare repository %q: %s", name, err)
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Repository().RemoveId(name); err != nil {
		if err == mgo.ErrNotFound {
			return ErrRepositoryNotFound
		}
		return err
	}
	return nil
}

// Update update a repository data.
func Update(name string, newData Repository) error {
	log.Debugf("Updating repository %q data", name)
	repo, err := Get(name)
	if err != nil {
		log.Errorf("repository.Update(%q): %s", name, err)
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(newData.Name) > 0 && newData.Name != repo.Name {
		oldName := repo.Name
		log.Debugf("Renaming repository %q to %q", oldName, newData.Name)
		err = conn.Repository().Insert(newData)
		if err != nil {
			log.Errorf("repository.Rename: Error adding new repository %q: %s", newData.Name, err)
			return err
		}
		err = conn.Repository().RemoveId(oldName)
		if err != nil {
			log.Errorf("repository.Rename: Error removing old repository %q: %s", oldName, err)
			return err
		}
		err = fs.Filesystem().Rename(barePath(oldName), barePath(newData.Name))
		if err != nil {
			log.Errorf("repository.Rename: Error renaming old repository in filesystem %q: %s", oldName, err)
			return err
		}
	} else {
		err = conn.Repository().UpdateId(repo.Name, newData)
		if err != nil {
			log.Errorf("repository.Update: Error updating repository data %q: %s", repo.Name, err)
			return err
		}
	}
	return nil
}

// ReadWriteURL formats the git ssh url and return it. If no remote is configured in
// gandalf.conf, this method panics.
func (r *Repository) ReadWriteURL() string {
	uid, err := config.GetString("uid")
	if err != nil {
		panic(err.Error())
	}
	remote := uid + "@%s:%s.git"
	if useSSH, _ := config.GetBool("git:ssh:use"); useSSH {
		var port string
		port, err = config.GetString("git:ssh:port")
		if err == nil {
			remote = "ssh://" + uid + "@%s:" + port + "/%s.git"
		} else {
			remote = "ssh://" + uid + "@%s/%s.git"
		}
	}
	host, err := config.GetString("host")
	if err != nil {
		panic(err.Error())
	}
	return fmt.Sprintf(remote, host, r.Name)
}

// ReadOnly formats the git url and return it. If no host is configured in
// gandalf.conf, this method panics.
func (r *Repository) ReadOnlyURL() string {
	remote := "git://%s/%s.git"
	if useSSH, _ := config.GetBool("git:ssh:use"); useSSH {
		uid, err := config.GetString("uid")
		if err != nil {
			panic(err.Error())
		}
		port, err := config.GetString("git:ssh:port")
		if err == nil {
			remote = "ssh://" + uid + "@%s:" + port + "/%s.git"
		} else {
			remote = "ssh://" + uid + "@%s/%s.git"
		}
	}
	host, err := config.GetString("readonly-host")
	if err != nil {
		host, err = config.GetString("host")
		if err != nil {
			panic(err)
		}
	}
	return fmt.Sprintf(remote, host, r.Name)
}

// Validates a repository
// A valid repository MUST have:
//  - a name without any special chars only alphanumeric and underlines are allowed.
//  - at least one user in users array
// A valid repository MAY have one namespace since the following is obeyed:
//  - a namespace is optional
//  - a namespace contains only alphanumerics, underlines, @´s, -´s, +´s and
//    periods but it does not start with a period (.)
//  - one and exactly one slash (/) separates namespace and the actual name
func (r *Repository) isValid() (bool, error) {
	// The following regex validates the name of a repository, which may
	// contain a namespace. If a namespace is used, we validate it
	// accordingly (see comments above)
	m, e := regexp.Match(`^([\w-+@][\w-+.@]*/)?[\w-]+$`, []byte(r.Name))
	if e != nil {
		panic(e)
	}
	if !m {
		return false, &InvalidRepositoryError{message: "repository name is not valid"}
	}
	absPath, err := filepath.Abs(barePath(r.Name))
	if err != nil || !strings.HasPrefix(absPath, bare) {
		return false, &InvalidRepositoryError{message: "repository name is not valid"}
	}
	if len(r.Users) == 0 {
		return false, &InvalidRepositoryError{message: "repository should have at least one user"}
	}
	return true, nil
}

// GrantAccess gives full or read-only permission for users in all specified repositories.
// If any of the repositories/users does not exist, GrantAccess just skips it.
func GrantAccess(rNames, uNames []string, readOnly bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var info *mgo.ChangeInfo
	if readOnly {
		info, err = conn.Repository().UpdateAll(bson.M{"_id": bson.M{"$in": rNames}}, bson.M{"$addToSet": bson.M{"readonlyusers": bson.M{"$each": uNames}}})
	} else {
		info, err = conn.Repository().UpdateAll(bson.M{"_id": bson.M{"$in": rNames}}, bson.M{"$addToSet": bson.M{"users": bson.M{"$each": uNames}}})
	}
	if err != nil {
		return err
	}
	if info.Updated < 1 {
		return ErrRepositoryNotFound
	}
	return nil
}

// RevokeAccess revokes write permission from users in all specified
// repositories.
func RevokeAccess(rNames, uNames []string, readOnly bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var info *mgo.ChangeInfo
	if readOnly {
		info, err = conn.Repository().UpdateAll(bson.M{"_id": bson.M{"$in": rNames}}, bson.M{"$pullAll": bson.M{"readonlyusers": uNames}})
	} else {
		info, err = conn.Repository().UpdateAll(bson.M{"_id": bson.M{"$in": rNames}}, bson.M{"$pullAll": bson.M{"users": uNames}})
	}
	if err != nil {
		return err
	}
	if info.Updated < 1 {
		return ErrRepositoryNotFound
	}
	return nil
}

func GetArchiveUrl(repo, ref, format string) string {
	url := "/repository/%s/archive?ref=%s&format=%s"
	return fmt.Sprintf(url, repo, ref, format)
}

type ArchiveFormat int

const (
	Zip ArchiveFormat = iota
	Tar
	TarGz
)

type ContentRetriever interface {
	GetContents(repo, ref, path string) ([]byte, error)
	GetArchive(repo, ref string, format ArchiveFormat) ([]byte, error)
	GetTree(repo, ref, path string) ([]map[string]string, error)
	GetForEachRef(repo, pattern string) ([]Ref, error)
	GetBranches(repo string) ([]Ref, error)
	GetDiff(repo, lastCommit, previousCommit string) ([]byte, error)
	GetTags(repo string) ([]Ref, error)
	TempClone(repo string) (string, func(), error)
	Checkout(cloneDir, branch string, isNew bool) error
	AddAll(cloneDir string) error
	Commit(cloneDir, message string, author, committer GitUser) error
	Push(cloneDir, branch string) error
	CommitZip(repo string, z *multipart.FileHeader, c GitCommit) (*Ref, error)
	GetLogs(repo, hash string, total int, path string) (*GitHistory, error)
}

var Retriever ContentRetriever

type GitContentRetriever struct{}

func (*GitContentRetriever) GetContents(repo, ref, path string) ([]byte, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain file %s on ref %s of repository %s (%s).", path, ref, repo, err)
	}
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain file %s on ref %s of repository %s (Repository does not exist).", path, ref, repo)
	}
	cmd := exec.Command(gitPath, "show", fmt.Sprintf("%s:%s", ref, path))
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain file %s on ref %s of repository %s (%s).", path, ref, repo, err)
	}
	return out, nil
}

func (*GitContentRetriever) GetArchive(repo, ref string, format ArchiveFormat) ([]byte, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain archive for ref %s of repository %s (%s).", ref, repo, err)
	}
	var archiveFormat string
	switch format {
	case Tar:
		archiveFormat = "--format=tar"
	case TarGz:
		archiveFormat = "--format=tar.gz"
	default:
		archiveFormat = "--format=zip"
	}
	prefix := fmt.Sprintf("--prefix=%s-%s/", repo, ref)
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain archive for ref %s of repository %s (Repository does not exist).", ref, repo)
	}
	cmd := exec.Command(gitPath, "archive", ref, prefix, archiveFormat)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain archive for ref %s of repository %s (%s).", ref, repo, err)
	}
	return out, nil
}

func (*GitContentRetriever) GetTree(repo, ref, path string) ([]map[string]string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain tree %s on ref %s of repository %s (%s).", path, ref, repo, err)
	}
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain tree %s on ref %s of repository %s (Repository does not exist).", path, ref, repo)
	}
	cmd := exec.Command(gitPath, "ls-tree", "-r", ref, path)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain tree %s on ref %s of repository %s (%s).", path, ref, repo, err)
	}
	lines := strings.Split(string(out), "\n")
	objectCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		objectCount++
	}
	objects := make([]map[string]string, objectCount)
	objectCount = 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		tabbed := strings.Split(line, "\t")
		meta, filepath := tabbed[0], tabbed[1]
		meta_parts := strings.Split(meta, " ")
		permission, filetype, hash := meta_parts[0], meta_parts[1], meta_parts[2]
		object := make(map[string]string)
		object["permission"] = permission
		object["filetype"] = filetype
		object["hash"] = hash
		object["path"] = strings.TrimSpace(strings.Trim(filepath, "\""))
		object["rawPath"] = filepath
		objects[objectCount] = object
		objectCount++
	}
	return objects, nil
}

func (*GitContentRetriever) GetForEachRef(repo, pattern string) ([]Ref, error) {
	var ref, name, committerName, committerEmail, committerDate, authorName, authorEmail, authorDate, taggerName, taggerEmail, taggerDate, subject string
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain the refs of repository %s (%s).", repo, err)
	}
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain the refs of repository %s (Repository does not exist).", repo)
	}
	format := "%(objectname)%09%(refname:short)%09%(committername)%09%(committeremail)%09%(committerdate)%09%(authorname)%09%(authoremail)%09%(authordate)%09%(taggername)%09%(taggeremail)%09%(taggerdate)%09%(contents:subject)"
	cmd := exec.Command(gitPath, "for-each-ref", "--sort=-committerdate", "--format", format)
	if len(pattern) > 0 {
		cmd.Args = append(cmd.Args, pattern)
	}
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain the refs of repository %s (%s).", repo, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	objectCount := len(lines)
	if len(lines) == 1 && len(lines[0]) == 0 {
		objectCount = 0
	}
	objects := make([]Ref, objectCount)
	objectCount = 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) > 10 { // let there be commits with empty subject
			ref = fields[0]
			name = fields[1]
			committerName = fields[2]
			committerEmail = fields[3]
			committerDate = fields[4]
			authorName = fields[5]
			authorEmail = fields[6]
			authorDate = fields[7]
			taggerName = fields[8]
			taggerEmail = fields[9]
			taggerDate = fields[10]
			subject = strings.Join(fields[11:], "\t") // let there be subjects with \t
		} else {
			log.Errorf("Could not match required ref elements for %s (invalid git input: %s).", repo, line)
			continue
		}
		object := Ref{}
		object.Ref = ref
		object.Name = name
		object.Subject = subject
		if taggerDate != "" {
			object.CreatedAt = taggerDate
		} else {
			object.CreatedAt = authorDate
		}
		object.Committer = &GitUser{
			Name:  committerName,
			Email: committerEmail,
			Date:  committerDate,
		}
		object.Author = &GitUser{
			Name:  authorName,
			Email: authorEmail,
			Date:  authorDate,
		}
		object.Tagger = &GitUser{
			Name:  taggerName,
			Email: taggerEmail,
			Date:  taggerDate,
		}
		object.Links = &Links{
			ZipArchive: GetArchiveUrl(repo, name, "zip"),
			TarArchive: GetArchiveUrl(repo, name, "tar.gz"),
		}
		objects[objectCount] = object
		objectCount++
	}
	return objects[0:objectCount], nil
}

func (*GitContentRetriever) GetBranches(repo string) ([]Ref, error) {
	branches, err := retriever().GetForEachRef(repo, "refs/heads/")
	return branches, err
}

func (*GitContentRetriever) GetDiff(repo, previousCommit, lastCommit string) ([]byte, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain diff with commits %s and %s of repository %s (%s).", lastCommit, previousCommit, repo, err)
	}
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain diff with commits %s and %s of repository %s (Repository does not exist).", lastCommit, previousCommit, repo)
	}
	cmd := exec.Command(gitPath, "diff", previousCommit, lastCommit)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain diff with commits %s and %s of repository %s (%s).", lastCommit, previousCommit, repo, err)
	}
	return out, nil
}

func (*GitContentRetriever) GetTags(repo string) ([]Ref, error) {
	return retriever().GetForEachRef(repo, "refs/tags/")
}

func (*GitContentRetriever) TempClone(repo string) (cloneDir string, cleanUp func(), err error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", nil, fmt.Errorf("Error when trying to clone repository %s (%s).", repo, err)
	}
	repoDir := barePath(repo)
	repoExists, err := exists(repoDir)
	if err != nil || !repoExists {
		return "", nil, fmt.Errorf("Error when trying to clone repository %s (Repository does not exist).", repo)
	}
	cloneDir, err = ioutil.TempDir(tempDir, "gandalf_clone")
	if err != nil {
		return "", nil, fmt.Errorf("Error when trying to clone repository %s (Could not create temporary directory).", repo)
	}
	cleanUp = func() {
		os.RemoveAll(cloneDir)
	}
	cmd := exec.Command(gitPath, "clone", repoDir, cloneDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return cloneDir, cleanUp, fmt.Errorf("Error when trying to clone repository %s into %s (%s [%s]).", repo, cloneDir, err, out)
	}
	return cloneDir, cleanUp, nil
}

func (*GitContentRetriever) Checkout(cloneDir, branch string, isNew bool) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("Error when trying to checkout clone %s into branch %s (%s).", cloneDir, branch, err)
	}
	cloneExists, err := exists(cloneDir)
	if err != nil || !cloneExists {
		return fmt.Errorf("Error when trying to checkout clone %s into branch %s (Clone does not exist).", cloneDir, branch)
	}
	cmd := exec.Command(gitPath, "checkout")
	if isNew {
		cmd.Args = append(cmd.Args, "-b")
	}
	cmd.Args = append(cmd.Args, branch)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error when trying to checkout clone %s into branch %s (%s [%s]).", cloneDir, branch, err, out)
	}
	return nil
}

func (*GitContentRetriever) AddAll(cloneDir string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("Error when trying to add all to clone %s (%s).", cloneDir, err)
	}
	cloneExists, err := exists(cloneDir)
	if err != nil || !cloneExists {
		return fmt.Errorf("Error when trying to add all to clone %s (Clone does not exist).", cloneDir)
	}
	cmd := exec.Command(gitPath, "add", "--all")
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error when trying to add all to clone %s (%s [%s]).", cloneDir, err, out)
	}
	return nil
}

func (*GitContentRetriever) Commit(cloneDir, message string, author, committer GitUser) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("Error when trying to commit to clone %s (%s).", cloneDir, err)
	}
	cloneExists, err := exists(cloneDir)
	if err != nil || !cloneExists {
		return fmt.Errorf("Error when trying to commit to clone %s (Clone does not exist).", cloneDir)
	}
	cmd := exec.Command(gitPath, "commit", "-m", message, "--author", author.String(), "--allow-empty-message")
	env := os.Environ()
	env = append(env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", committer.Name))
	env = append(env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", committer.Email))
	cmd.Env = env
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error when trying to commit to clone %s (%s [%s]).", cloneDir, err, out)
	}
	return nil
}

func (*GitContentRetriever) Push(cloneDir, branch string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("Error when trying to push clone %s (%s).", cloneDir, err)
	}
	cloneExists, err := exists(cloneDir)
	if err != nil || !cloneExists {
		return fmt.Errorf("Error when trying to push clone %s into origin's %s branch (Clone does not exist).", cloneDir, branch)
	}
	cmd := exec.Command(gitPath, "push", "origin", branch)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error when trying to push clone %s into origin's %s branch (%s [%s]).", cloneDir, branch, err, out)
	}
	return nil
}

func (*GitContentRetriever) CommitZip(repo string, z *multipart.FileHeader, c GitCommit) (*Ref, error) {
	cloneDir, cleanUp, err := TempClone(repo)
	if cleanUp != nil {
		defer cleanUp()
	}
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not clone: %s", repo, err)
	}
	err = Checkout(cloneDir, c.Branch, false)
	if err != nil {
		err = Checkout(cloneDir, c.Branch, true)
		if err != nil {
			return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not checkout: %s", repo, err)
		}
	}
	err = multipartzip.ExtractZip(z, cloneDir)
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not extract: %s", repo, err)
	}
	err = AddAll(cloneDir)
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not add all: %s", repo, err)
	}
	err = Commit(cloneDir, c.Message, c.Author, c.Committer)
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not commit: %s", repo, err)
	}
	err = Push(cloneDir, c.Branch)
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not push: %s", repo, err)
	}
	branches, err := GetBranches(repo)
	if err != nil {
		return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not get branches: %s", repo, err)
	}
	for _, branch := range branches {
		if branch.Name == c.Branch {
			return &branch, nil
		}
	}
	return nil, fmt.Errorf("Error when trying to commit zip to repository %s, could not check branch: %s", repo, err)
}

func (*GitContentRetriever) GetLogs(repo, hash string, total int, path string) (*GitHistory, error) {
	if hash == "" {
		hash = "master"
	}
	if total < 1 {
		total = 1
	}
	totalPagination := total + 1
	var last string
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain the log of repository %s (%s).", repo, err)
	}
	cwd := barePath(repo)
	repoExists, err := exists(cwd)
	if err != nil || !repoExists {
		return nil, fmt.Errorf("Error when trying to obtain the log of repository %s (Repository does not exist).", repo)
	}
	format := "%H%x09%an%x09%ae%x09%ad%x09%cn%x09%ce%x09%cd%x09%P%x09%s"
	cmd := exec.Command(gitPath, "--no-pager", "log", fmt.Sprintf("-n %d", totalPagination), fmt.Sprintf("--format=%s", format), hash, "--", path)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Error when trying to obtain the log of repository %s (%s).", repo, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	objectCount := len(lines)
	if len(lines) == 1 && len(lines[0]) == 0 {
		objectCount = 0
	}
	if objectCount > total {
		last = lines[objectCount-1]
		lines = lines[0 : objectCount-1]
		objectCount -= 1
	}
	history := GitHistory{}
	commits := make([]GitLog, objectCount)
	objectCount = 0
	for _, line := range lines {
		var parent, subject string
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 { // let there be commits with empty subject and no parents
			return nil, fmt.Errorf("Error when trying to obtain the log of repository %s (Invalid git log output [%s]).", repo, out)
		}
		if len(fields) > 8 {
			parent = fields[7]
			subject = strings.Join(fields[8:], "\t") // let there be subjects with \t
		}
		commit := GitLog{}
		commit.Ref = fields[0]
		commit.Subject = subject
		commit.CreatedAt = fields[3]
		commit.Committer = &GitUser{
			Name:  fields[4],
			Email: fields[5],
			Date:  fields[6],
		}
		commit.Author = &GitUser{
			Name:  fields[1],
			Email: fields[2],
			Date:  fields[3],
		}
		if len(parent) > 0 {
			parents := strings.Split(parent, " ")
			parentCount := len(parents)
			aux := make([]string, parentCount)
			parentCount = 0
			for _, item := range parents {
				aux[parentCount] = item
				parentCount++
			}
			commit.Parent = aux
		}
		commits[objectCount] = commit
		objectCount++
	}
	history.Commits = commits
	if last != "" {
		fields := strings.Split(last, "\t")
		history.Next = fields[0]
	} else {
		history.Next = ""
	}
	return &history, nil
}

func retriever() ContentRetriever {
	if Retriever == nil {
		Retriever = &GitContentRetriever{}
	}
	return Retriever
}

// GetFileContents returns the contents for a given file
// in a given ref for the specified repository
func GetFileContents(repo, ref, path string) ([]byte, error) {
	return retriever().GetContents(repo, ref, path)
}

// GetArchive returns the contents for a given file
// in a given ref for the specified repository
func GetArchive(repo, ref string, format ArchiveFormat) ([]byte, error) {
	return retriever().GetArchive(repo, ref, format)
}

func GetTree(repo, ref, path string) ([]map[string]string, error) {
	return retriever().GetTree(repo, ref, path)
}

func GetForEachRef(repo, pattern string) ([]Ref, error) {
	return retriever().GetForEachRef(repo, pattern)
}

func GetBranches(repo string) ([]Ref, error) {
	return retriever().GetBranches(repo)
}

func GetDiff(repo, previousCommit, lastCommit string) ([]byte, error) {
	return retriever().GetDiff(repo, previousCommit, lastCommit)
}

func GetTags(repo string) ([]Ref, error) {
	return retriever().GetTags(repo)
}

func TempClone(repo string) (string, func(), error) {
	return retriever().TempClone(repo)
}

func Checkout(cloneDir, branch string, isNew bool) error {
	return retriever().Checkout(cloneDir, branch, isNew)
}

func AddAll(cloneDir string) error {
	return retriever().AddAll(cloneDir)
}

func Commit(cloneDir, message string, author, committer GitUser) error {
	return retriever().Commit(cloneDir, message, author, committer)
}

func Push(cloneDir, branch string) error {
	return retriever().Push(cloneDir, branch)
}

func CommitZip(repo string, z *multipart.FileHeader, c GitCommit) (*Ref, error) {
	return retriever().CommitZip(repo, z, c)
}

func GetLogs(repo, hash string, total int, path string) (*GitHistory, error) {
	return retriever().GetLogs(repo, hash, total, path)
}

type InvalidRepositoryError struct {
	message string
}

func (err *InvalidRepositoryError) Error() string {
	return err.message
}
