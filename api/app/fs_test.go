package app

import (
	"github.com/timeredbull/tsuru/fs"
	"os"
)

type RecordingFs struct {
	actions []string
}

func (r *RecordingFs) HasAction(action string) bool {
	for _, a := range r.actions {
		if action == a {
			return true
		}
	}
	return false
}

func (r *RecordingFs) Create(name string) (fs.File, error) {
	return nil, nil
}

func (r *RecordingFs) Mkdir(name string, perm os.FileMode) error {
	return nil
}

func (r *RecordingFs) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (r *RecordingFs) Open(name string) (fs.File, error) {
	return nil, nil
}

func (r *RecordingFs) Remove(name string) error {
	r.actions = append(r.actions, "remove "+name)
	return nil
}

func (r *RecordingFs) RemoveAll(path string) error {
	r.actions = append(r.actions, "removeall "+path)
	return nil
}

func (r *RecordingFs) Stat(name string) (os.FileInfo, error) {
	return nil, nil
}
