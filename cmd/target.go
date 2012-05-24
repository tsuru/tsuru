package cmd

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
)

type Target struct{}

func (t *Target) Info() *Info {
	return &Info{
		Name:  "target",
		Usage: "target <target>",
		Desc:  "Defines the target (tsuru server)",
		Args:  1,
	}
}

func (t *Target) Run(ctx *Context, client Doer) error {
	target := ctx.Args[0]
	err := WriteTarget(target)
	if err != nil {
		return err
	}
	io.WriteString(ctx.Stdout, "New target is "+target+"\n")
	return nil
}

const DefaultTarget = "http://tsuru.plataformas.glb.com:8080"

func ReadTarget() string {
	targetPath, _ := joinWithUserDir(".tsuru_target")
	if f, err := os.Open(targetPath); err == nil {
		defer f.Close()
		if b, err := ioutil.ReadAll(f); err == nil {
			return string(b)
		}
	}
	return DefaultTarget
}

func GetUrl(path string) string {
	return ReadTarget() + path
}

func WriteTarget(t string) error {
	targetPath, err := joinWithUserDir(".tsuru_target")
	if err != nil {
		return err
	}
	targetFile, err := os.OpenFile(targetPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0600)
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
