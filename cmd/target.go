// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

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

func readTarget() (string, error) {
	targetPath := joinWithUserDir(".tsuru_target")
	if f, err := filesystem().Open(targetPath); err == nil {
		defer f.Close()
		if b, err := ioutil.ReadAll(f); err == nil {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", undefinedTargetError{}
}

func deleteTargetFile() {
	filesystem().Remove(joinWithUserDir(".tsuru_target"))
}

func GetURL(path string) (string, error) {
	var prefix string
	target, err := readTarget()
	if err != nil {
		return "", err
	}
	if m, _ := regexp.MatchString("^https?://", target); !m {
		prefix = "http://"
	}
	return prefix + strings.TrimRight(target, "/") + path, nil
}

func writeTarget(t string) error {
	targetPath := joinWithUserDir(".tsuru_target")
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
	err := writeOnTargetList(label, target)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "New target %s -> %s added to target list", label, target)
	if t.set {
		writeTarget(target)
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
	targetsPath := joinWithUserDir(".tsuru_targets")
	targetsFile, err := filesystem().OpenFile(targetsPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer targetsFile.Close()
	return nil
}

func writeOnTargetList(label string, target string) error {
	label = strings.TrimSpace(label)
	target = strings.TrimSpace(target)
	targetExist, err := checkIfTargetLabelExists(label)
	if err != nil {
		return err
	}
	if targetExist {
		return errors.New("Target label provided already exist")
	}
	targetsPath := joinWithUserDir(".tsuru_targets")
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

func checkIfTargetLabelExists(label string) (bool, error) {
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
	targetsPath := joinWithUserDir(".tsuru_targets")
	if f, err := filesystem().Open(targetsPath); err == nil {
		defer f.Close()
		if b, err := ioutil.ReadAll(f); err == nil {
			var targetLines = strings.Split(strings.TrimSpace(string(b)), "\n")

			for i := range targetLines {
				var targetSplt = strings.Split(targetLines[i], "\t")

				if len(targetSplt) == 2 {
					targets[targetSplt[0]] = targetSplt[1]
				}
			}
		}
	}
	return targets, nil
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
	if current, err := readTarget(); err == nil {
		slice.setCurrent(current)
	}
	fmt.Fprintf(ctx.Stdout, "%s\n", slice)
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
		if current, err := readTarget(); err == nil && current == turl {
			deleteTargetFile()
		}
	}
	err = resetTargetList()
	if err != nil {
		return err
	}
	for label, target := range targets {
		writeOnTargetList(label, target)
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
	labelExist, err := checkIfTargetLabelExists(targetLabelToSet)
	if !labelExist {
		return errors.New("Target not found")
	}
	targets, err := getTargets()
	if err != nil {
		return err
	}
	for label, target := range targets {
		if label == targetLabelToSet {
			err = writeTarget(target)
			if err != nil {
				return err
			}
			fmt.Fprintf(ctx.Stdout, "New target is %s -> %s\n", label, target)
		}
	}
	return nil
}

type undefinedTargetError struct{}

func (t undefinedTargetError) Error() string {
	return `No target defined. Please use target-add/target-set to define a target.

For more details, please run "tsuru help target".`
}
