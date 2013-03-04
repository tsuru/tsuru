// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"syscall"
)

type target struct{}

func (t *target) Info() *Info {
	desc := `Defines or retrieve the target (tsuru server)

If an argument is provided, this command sets the target, otherwise it displays the current target.
`
	return &Info{
		Name:    "target",
		Usage:   "target [target]",
		Desc:    desc,
		MinArgs: 0,
	}
}

func (t *target) Run(ctx *Context, client Doer) error {
	var target string
	if len(ctx.Args) > 0 {
		target = ctx.Args[0]
		err := writeTarget(target)
		if err != nil {
			return err
		}
		fmt.Fprintf(ctx.Stdout, "New target is %s\n", target)
		return nil
	}
	target = readTarget()
	fmt.Fprintf(ctx.Stdout, "Current target is %s\n", target)
	return nil
}

const DefaultTarget = "http://tsuru.plataformas.glb.com:8080"

func readTarget() string {
	targetPath, _ := joinWithUserDir(".tsuru_target")
	if f, err := filesystem().Open(targetPath); err == nil {
		defer f.Close()
		if b, err := ioutil.ReadAll(f); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return DefaultTarget
}

func GetUrl(path string) string {
	var prefix string
	target := readTarget()
	if m, _ := regexp.MatchString("^https?://", target); !m {
		prefix = "http://"
	}
	return prefix + target + path
}

func writeTarget(t string) error {
	targetPath, err := joinWithUserDir(".tsuru_target")
	if err != nil {
		return err
	}
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

type targetAdd struct{}

func (t *targetAdd) Info() *Info {
	desc := `Add a new target on target-list (tsuru server)
`
	return &Info{
		Name:    "target-add",
		Usage:   "target-add <label> <target>",
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

	label = strings.TrimSpace(ctx.Args[0])
	target = strings.TrimSpace(ctx.Args[1])

	targetExist, err := checkIfTargetLabelExists(label)
	if err != nil {
		return err
	}
	if targetExist {
		return errors.New("Target label provided already exist")
	}

	targetsPath, err := joinWithUserDir(".tsuru_targets")
	if err != nil {
		return err
	}

	targetsFile, err := filesystem().OpenFile(targetsPath, syscall.O_RDWR|syscall.O_CREAT|syscall.O_APPEND, 0600)
	if err != nil {
		return err
	}

	defer targetsFile.Close()
	var content = label + "\t" + target + "\n"
	n, err := targetsFile.WriteString(content)
	if n != len(content) || err != nil {
		return errors.New("Failed to write the target file")
	}

	fmt.Fprintf(ctx.Stdout, "New target %s -> %s added to target-list\n", label, target)

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

	targetsPath, err := joinWithUserDir(".tsuru_targets")
	if err != nil {
		return targets, err
	}

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

	fmt.Fprintf(ctx.Stdout, "target-list not implemented")

	return nil

}
