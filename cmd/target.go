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
	"strings"
	"syscall"
)

type target struct{}

func (t *target) Info() *Info {
	desc := `Retrieve current target (tsuru server)

Displays the current target.
`
	return &Info{
		Name:    "target",
		Usage:   "target",
		Desc:    desc,
		MinArgs: 0,
	}
}

func (t *target) Run(ctx *Context, client Doer) error {
	var target string
	if len(ctx.Args) > 0 {
		fmt.Fprintf(ctx.Stdout, "To add a new target use target-add\n")
		return nil
	}
	target, _ = readTarget()
	fmt.Fprintf(ctx.Stdout, "Current target is %s\n", target)
	return nil
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

func GetUrl(path string) (string, error) {
	var prefix string
	target, err := readTarget()
	if err != nil {
		return "", err
	}
	if m, _ := regexp.MatchString("^https?://", target); !m {
		prefix = "http://"
	}
	return prefix + target + path, nil
}

func writeTarget(t string) error {
	targetPath := joinWithUserDir(".tsuru_target")
	targetFile, err := filesystem().OpenFile(targetPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer targetFile.Close()
	t = strings.TrimRight(t, "/")
	content := []byte(t)
	n, err := targetFile.Write(content)
	if n != len(content) || err != nil {
		return errors.New("Failed to write the target file")
	}
	return nil
}

type targetAdd struct {
	fs  *gnuflag.FlagSet
	set bool
}

func (t *targetAdd) Info() *Info {
	desc := `Add a new target on target-list (tsuru server)
`
	return &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target> [--set-current]",
		Desc:    desc,
		MinArgs: 2,
	}
}

func (t *targetAdd) Run(ctx *Context, client Doer) error {
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
	fmt.Fprintf(ctx.Stdout, "New target %s -> %s added to target-list\n", label, target)
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
	desc := `List all targets (tsuru server)
`
	return &Info{
		Name:    "target-list",
		Usage:   "target-list",
		Desc:    desc,
		MinArgs: 0,
	}
}

func (t *targetList) Run(ctx *Context, client Doer) error {
	table := NewTable()
	targets, err := getTargets()
	if err != nil {
		return err
	}
	for label, target := range targets {
		table.AddRow(Row{label, target})
	}
	table.Sort()
	fmt.Fprint(ctx.Stdout, table)
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

func (t *targetRemove) Run(ctx *Context, client Doer) error {
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

func (t *targetSet) Run(ctx *Context, client Doer) error {
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
