// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

var errUndefinedTarget = errors.New(`No target defined. Please use target-add/target-set to define a target.

For more details, please run "tsuru help target".`)

type tsuruTarget struct {
	label, url string
}

func (t *tsuruTarget) String() string {
	return t.label + " (" + t.url + ")"
}

type targetSlice struct {
	targets []tsuruTarget
	current int
	sorted  bool
}

func newTargetSlice() *targetSlice {
	return &targetSlice{current: -1}
}

func (t *targetSlice) add(label, url string) {
	t.targets = append(t.targets, tsuruTarget{label: label, url: url})
	length := t.Len()
	if length > 1 && !t.Less(t.Len()-2, t.Len()-1) {
		t.sorted = false
	}
}

func (t *targetSlice) Len() int {
	return len(t.targets)
}

func (t *targetSlice) Less(i, j int) bool {
	return t.targets[i].label < t.targets[j].label
}

func (t *targetSlice) Swap(i, j int) {
	t.targets[i], t.targets[j] = t.targets[j], t.targets[i]
}

func (t *targetSlice) Sort() {
	sort.Sort(t)
	t.sorted = true
}

func (t *targetSlice) setCurrent(url string) {
	if !t.sorted {
		t.Sort()
	}
	for i, target := range t.targets {
		if target.url == url {
			t.current = i
			break
		}
	}
}

func (t *targetSlice) String() string {
	if !t.sorted {
		t.Sort()
	}
	values := make([]string, len(t.targets))
	for i, target := range t.targets {
		prefix := "  "
		if t.current == i {
			prefix = "* "
		}
		values[i] = prefix + target.String()
	}
	return strings.Join(values, "\n")
}

// ReadTarget returns the current target, as defined in the TSURU_TARGET
// environment variable or in the target file.
func ReadTarget() (string, error) {
	if target := os.Getenv("TSURU_TARGET"); target != "" {
		targets, err := getTargets()
		if err == nil {
			if val, ok := targets[target]; ok {
				return val, nil
			}
		}
		return target, nil
	}
	targetPath := JoinWithUserDir(".tsuru", "target")
	target, err := readTarget(targetPath)
	if err == errUndefinedTarget {
		copyTargetFiles()
		target, err = readTarget(JoinWithUserDir(".tsuru_target"))
	}
	return target, err
}

func readTarget(targetPath string) (string, error) {
	if f, err := filesystem().Open(targetPath); err == nil {
		defer f.Close()
		if b, err := io.ReadAll(f); err == nil {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", errUndefinedTarget
}

func deleteTargetFile() {
	filesystem().Remove(JoinWithUserDir(".tsuru", "target"))
}

func GetTarget() (string, error) {
	var prefix string
	target, err := ReadTarget()
	if err != nil {
		return "", err
	}
	if m, _ := regexp.MatchString("^https?://", target); !m {
		prefix = "http://"
	}
	return prefix + target, nil
}

func GetTargetLabel() (string, error) {
	target, err := GetTarget()
	if err != nil {
		return "", err
	}
	targets, err := getTargets()
	if err != nil {
		return "", err
	}
	targetKeys := make([]string, len(targets))
	for k := range targets {
		targetKeys = append(targetKeys, k)
	}
	sort.Strings(targetKeys)
	for _, k := range targetKeys {
		if targets[k] == target {
			return k, nil
		}
	}
	return "", errors.Errorf("label for target %q not found ", target)
}

func GetURLVersion(version, path string) (string, error) {
	target, err := GetTarget()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(target, "/") + "/" + version + path, nil
}

func GetURL(path string) (string, error) {
	return GetURLVersion("1.0", path)
}

// WriteTarget writes the given endpoint to the target file.
func WriteTarget(t string) error {
	targetPath := JoinWithUserDir(".tsuru", "target")
	targetFile, err := filesystem().OpenFile(targetPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer targetFile.Close()
	n, err := targetFile.WriteString(t)
	if n != len(t) || err != nil {
		return errors.New("Failed to write the target file")
	}
	return nil
}

func resetTargetList() error {
	targetsPath := JoinWithUserDir(".tsuru", "targets")
	targetsFile, err := filesystem().OpenFile(targetsPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer targetsFile.Close()
	return nil
}

// WriteOnTargetList writes the given target in the target list file.
func WriteOnTargetList(label, target string) error {
	label = strings.TrimSpace(label)
	target = strings.TrimSpace(target)
	targetExist, err := CheckIfTargetLabelExists(label)
	if err != nil {
		return err
	}
	if targetExist {
		return errors.New("Target label provided already exists")
	}
	targetsPath := JoinWithUserDir(".tsuru", "targets")
	targetsFile, err := filesystem().OpenFile(targetsPath, syscall.O_RDWR|syscall.O_CREAT|syscall.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer targetsFile.Close()
	content := label + "\t" + target + "\n"
	n, err := targetsFile.WriteString(content)
	if n != len(content) || err != nil {
		return errors.New("Failed to write the target file")
	}
	return nil
}

func CheckIfTargetLabelExists(label string) (bool, error) {
	targets, err := getTargets()
	if err != nil {
		return false, err
	}
	_, exists := targets[label]
	if exists {
		return true, nil
	}
	return false, nil
}

func getTargets() (map[string]string, error) {
	var targets = map[string]string{}
	legacyTargetsPath := JoinWithUserDir(".tsuru_targets")
	targetsPath := JoinWithUserDir(".tsuru", "targets")
	err := filesystem().MkdirAll(JoinWithUserDir(".tsuru"), 0700)
	if err != nil {
		return nil, err
	}
	var legacy bool
	f, err := filesystem().Open(targetsPath)
	if os.IsNotExist(err) {
		f, err = filesystem().Open(legacyTargetsPath)
		legacy = true
	}
	if err == nil {
		defer f.Close()
		if b, err := io.ReadAll(f); err == nil {
			var targetLines = strings.Split(strings.TrimSpace(string(b)), "\n")
			for i := range targetLines {
				var targetSplit = strings.Split(targetLines[i], "\t")

				if len(targetSplit) == 2 {
					targets[targetSplit[0]] = targetSplit[1]
				}
			}
		}
	}
	if legacy {
		copyTargetFiles()
	}
	return targets, nil
}

func copyTargetFiles() {
	filesystem().MkdirAll(JoinWithUserDir(".tsuru"), 0700)
	if src, err := filesystem().Open(JoinWithUserDir(".tsuru_targets")); err == nil {
		defer src.Close()
		if dst, err := filesystem().OpenFile(JoinWithUserDir(".tsuru", "targets"), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600); err == nil {
			defer dst.Close()
			io.Copy(dst, src)
		}
	}
	if target, err := readTarget(JoinWithUserDir(".tsuru_target")); err == nil {
		WriteTarget(target)
	}
}
