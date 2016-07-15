// Copyright 2014 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"time"
)

type MockContentRetriever struct {
	LastFormat     ArchiveFormat
	LastRef        string
	LastPath       string
	ResultContents []byte
	Tree           []map[string]string
	Ref            Ref
	Refs           []Ref
	LookPathError  error
	OutputError    error
	ClonePath      string
	CleanUp        func()
	History        GitHistory
}

func (r *MockContentRetriever) GetContents(repo, ref, path string) ([]byte, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	r.LastRef = ref
	return r.ResultContents, nil
}

func (r *MockContentRetriever) GetArchive(repo, ref string, format ArchiveFormat) ([]byte, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	r.LastRef = ref
	r.LastFormat = format
	return r.ResultContents, nil
}

func CreateEmptyFile(tmpPath, repo, file string) error {
	testPath := path.Join(tmpPath, repo+".git")
	if file == "" {
		file = fmt.Sprintf("README_%d", time.Now().UnixNano())
	}
	content := ""
	return ioutil.WriteFile(path.Join(testPath, file), []byte(content), 0644)
}

func CreateFolder(tmpPath, repo, folder string) (string, error) {
	testPath := path.Join(tmpPath, repo+".git")
	folderPath := path.Join(testPath, folder)
	err := os.MkdirAll(folderPath, 0777)
	return folderPath, err
}

func CreateFile(testPath, file, content string) error {
	now := time.Now().UnixNano()
	if file == "" {
		file = fmt.Sprintf("README_%d", now)
	}
	if content == "" {
		content = fmt.Sprintf("much WOW %d", now)
	}
	return ioutil.WriteFile(path.Join(testPath, file), []byte(content), 0644)
}

func AddAllMock(testPath string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "add", "--all", ".")
	cmd.Dir = testPath
	return cmd.Run()
}

func MakeCommit(testPath, content string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	err = AddAllMock(testPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "commit", "-m", content, "--allow-empty-message")
	cmd.Dir = testPath
	return cmd.Run()
}

func PushTags(testPath string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "push", "--tags")
	cmd.Dir = testPath
	return cmd.Run()
}

func CreateCommit(tmpPath, repo, file, content string) error {
	testPath := path.Join(tmpPath, repo+".git")
	err := CreateFile(testPath, file, content)
	if err != nil {
		return err
	}
	return MakeCommit(testPath, content)
}

func InitRepository(testPath string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "init")
	cmd.Dir = testPath
	err = cmd.Run()
	if err != nil {
		return err
	}
	err = CreateOrUpdateConfig(testPath, "user.email", "much@email.com")
	if err != nil {
		return err
	}
	return CreateOrUpdateConfig(testPath, "user.name", "doge")
}

func InitBareRepository(testPath string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "init", "--bare")
	cmd.Dir = testPath
	err = cmd.Run()
	if err != nil {
		return err
	}
	err = CreateOrUpdateConfig(testPath, "user.email", "much@email.com")
	if err != nil {
		return err
	}
	return CreateOrUpdateConfig(testPath, "user.name", "doge")
}

func CreateEmptyTestRepository(tmpPath, repo string) (func(), error) {
	testPath := path.Join(tmpPath, repo+".git")
	cleanup := func() {
		os.RemoveAll(testPath)
	}
	err := os.MkdirAll(testPath, 0777)
	if err != nil {
		return cleanup, err
	}
	err = InitRepository(testPath)
	return cleanup, err
}

func CreateEmptyTestBareRepository(tmpPath, repo string) (func(), error) {
	testPath := path.Join(tmpPath, repo+".git")
	cleanup := func() {
		os.RemoveAll(testPath)
	}
	err := os.MkdirAll(testPath, 0777)
	if err != nil {
		return cleanup, err
	}
	err = InitBareRepository(testPath)
	return cleanup, err
}

func CheckoutInNewBranch(testPath, branch string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "checkout", "-b", branch)
	cmd.Dir = testPath
	return cmd.Run()
}

func CreateOrUpdateConfig(testPath, param, value string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "config", param, value)
	cmd.Dir = testPath
	return cmd.Run()
}

func CreateTestRepository(tmpPath, repo, file, content string, folders ...string) (func(), error) {
	testPath := path.Join(tmpPath, repo+".git")
	cleanup := func() {
		os.RemoveAll(testPath)
	}
	err := os.MkdirAll(testPath, 0777)
	if err != nil {
		return cleanup, err
	}
	err = InitRepository(testPath)
	if err != nil {
		return cleanup, err
	}
	err = CreateFile(testPath, file, content)
	if err != nil {
		return cleanup, err
	}
	for _, folder := range folders {
		folderPath, createErr := CreateFolder(tmpPath, repo, folder)
		if createErr != nil {
			return cleanup, createErr
		}
		createErr = CreateFile(folderPath, file, content)
		if createErr != nil {
			return cleanup, createErr
		}
	}
	err = MakeCommit(testPath, content)
	return cleanup, err
}

func GetLastHashCommit(tmpPath, repo string) ([]byte, error) {
	testPath := path.Join(tmpPath, repo+".git")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(gitPath, "log", "--pretty=format:%H", "-1")
	cmd.Dir = testPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func StatusRepository(testPath string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "status")
	cmd.Dir = testPath
	return cmd.Run()
}

func CreateBranchesOnTestRepository(tmpPath string, repo string, branches ...string) error {
	testPath := path.Join(tmpPath, repo+".git")
	err := StatusRepository(testPath)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		err = CheckoutInNewBranch(testPath, branch)
		if err != nil {
			return err
		}
	}
	return err
}

func CreateTag(testPath, tagname string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "tag", tagname)
	cmd.Dir = testPath
	return cmd.Run()
}

func CreateAnnotatedTag(testPath, tagname, message string, tagger GitUser) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	cmd := exec.Command(gitPath, "tag", "-a", tagname, "-m", message)
	env := os.Environ()
	env = append(env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", tagger.Name))
	env = append(env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", tagger.Email))
	cmd.Env = env
	cmd.Dir = testPath
	return cmd.Run()
}

func (r *MockContentRetriever) GetTree(repo, ref, path string) ([]map[string]string, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	r.LastRef = ref
	r.LastPath = path
	return r.Tree, nil
}

func (r *MockContentRetriever) GetForEachRef(repo, pattern string) ([]Ref, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return r.Refs, nil
}

func (r *MockContentRetriever) GetBranches(repo string) ([]Ref, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return r.Refs, nil
}

func (r *MockContentRetriever) GetDiff(repo, previousCommit, lastCommit string) ([]byte, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return r.ResultContents, nil
}

func (r *MockContentRetriever) GetTags(repo string) ([]Ref, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return r.Refs, nil
}

func (r *MockContentRetriever) TempClone(repo string) (string, func(), error) {
	if r.LookPathError != nil {
		return "", nil, r.LookPathError
	}
	if r.OutputError != nil {
		return "", nil, r.OutputError
	}
	return r.ClonePath, r.CleanUp, nil
}

func (r *MockContentRetriever) Checkout(cloneDir, branch string, isNew bool) error {
	if r.LookPathError != nil {
		return r.LookPathError
	}
	if r.OutputError != nil {
		return r.OutputError
	}
	return nil
}

func (r *MockContentRetriever) AddAll(cloneDir string) error {
	if r.LookPathError != nil {
		return r.LookPathError
	}
	if r.OutputError != nil {
		return r.OutputError
	}
	return nil
}

func (r *MockContentRetriever) Commit(cloneDir, message string, author, committer GitUser) error {
	if r.LookPathError != nil {
		return r.LookPathError
	}
	if r.OutputError != nil {
		return r.OutputError
	}
	return nil
}

func (r *MockContentRetriever) Push(cloneDir, branch string) error {
	if r.LookPathError != nil {
		return r.LookPathError
	}
	if r.OutputError != nil {
		return r.OutputError
	}
	return nil
}

func (r *MockContentRetriever) CommitZip(repo string, z *multipart.FileHeader, c GitCommit) (*Ref, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return &r.Ref, nil
}

func (r *MockContentRetriever) GetLogs(repo, hash string, total int, path string) (*GitHistory, error) {
	if r.LookPathError != nil {
		return nil, r.LookPathError
	}
	if r.OutputError != nil {
		return nil, r.OutputError
	}
	return &r.History, nil
}
