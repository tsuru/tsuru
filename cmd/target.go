// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/tsuru/gnuflag"
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
		if b, err := ioutil.ReadAll(f); err == nil {
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
	for k, v := range targets {
		if v == target {
			return k, nil
		}
	}
	return "", errors.New("label for target not found " + target)
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

type targetAdd struct {
	fs  *gnuflag.FlagSet
	set bool
}

func (t *targetAdd) Info() *Info {
	return &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target> [--set-current|-s]",
		Desc:    "Adds a new entry to the list of available targets",
		MinArgs: 2,
	}
}

func (t *targetAdd) Run(ctx *Context, client *Client) error {
	var target string
	var label string
	if len(ctx.Args) != 2 {
		return errors.New("Invalid arguments")
	}
	label = ctx.Args[0]
	target = ctx.Args[1]
	err := WriteOnTargetList(label, target)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "New target %s -> %s added to target list", label, target)
	if t.set {
		WriteTarget(target)
		fmt.Fprint(ctx.Stdout, " and defined as the current target")
	}
	fmt.Fprintln(ctx.Stdout)
	return nil
}

func (t *targetAdd) Flags() *gnuflag.FlagSet {
	if t.fs == nil {
		t.fs = gnuflag.NewFlagSet("target-add", gnuflag.ExitOnError)
		t.fs.BoolVar(&t.set, "set-current", false, "Add and define the target as the current target")
		t.fs.BoolVar(&t.set, "s", false, "Add and define the target as the current target")
	}
	return t.fs
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
		if b, err := ioutil.ReadAll(f); err == nil {
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

type targetList struct{}

func (t *targetList) Info() *Info {
	desc := `Displays the list of targets, marking the current.

Other commands related to target:

  - target-add: adds a new target to the list of targets
  - target-set: defines one of the targets in the list as the current target
  - target-remove: removes one target from the list`
	return &Info{
		Name:    "target-list",
		Usage:   "target-list",
		Desc:    desc,
		MinArgs: 0,
	}
}

func (t *targetList) Run(ctx *Context, client *Client) error {
	slice := newTargetSlice()
	targets, err := getTargets()
	if err != nil {
		return err
	}
	for label, target := range targets {
		slice.add(label, target)
	}
	if current, err := ReadTarget(); err == nil {
		slice.setCurrent(current)
	}
	fmt.Fprintf(ctx.Stdout, "%v\n", slice)
	return nil
}

type targetRemove struct{}

func (t *targetRemove) Info() *Info {
	desc := `Remove a target from target-list (tsuru server)
`
	return &Info{
		Name:    "target-remove",
		Usage:   "target-remove",
		Desc:    desc,
		MinArgs: 1,
	}
}

func (t *targetRemove) Run(ctx *Context, client *Client) error {
	if len(ctx.Args) != 1 {
		return errors.New("Invalid arguments")
	}
	targetLabelToRemove := strings.TrimSpace(ctx.Args[0])
	targets, err := getTargets()
	if err != nil {
		return err
	}
	var turl string
	for label, url := range targets {
		if label == targetLabelToRemove {
			turl = url
			delete(targets, label)
		}
	}
	if turl != "" {
		var current string
		if current, err = ReadTarget(); err == nil && current == turl {
			deleteTargetFile()
		}
	}
	err = resetTargetList()
	if err != nil {
		return err
	}
	for label, target := range targets {
		WriteOnTargetList(label, target)
	}
	return nil
}

type targetSet struct{}

func (t *targetSet) Info() *Info {
	desc := `Change current target (tsuru server)
`
	return &Info{
		Name:    "target-set",
		Usage:   "target-set <label>",
		Desc:    desc,
		MinArgs: 1,
	}
}

func (t *targetSet) Run(ctx *Context, client *Client) error {
	if len(ctx.Args) != 1 {
		return errors.New("Invalid arguments")
	}
	targetLabelToSet := strings.TrimSpace(ctx.Args[0])
	labelExist, err := CheckIfTargetLabelExists(targetLabelToSet)
	if err != nil {
		return err
	}
	if !labelExist {
		return errors.New("Target not found")
	}
	targets, err := getTargets()
	if err != nil {
		return err
	}
	for label, target := range targets {
		if label == targetLabelToSet {
			err = WriteTarget(target)
			if err != nil {
				return err
			}
			fmt.Fprintf(ctx.Stdout, "New target is %s -> %s\n", label, target)
		}
	}
	return nil
}
