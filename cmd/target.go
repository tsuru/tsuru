package cmd

import (
	"errors"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"syscall"
)

type Target struct{}

func (t *Target) Info() *Info {
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

func (t *Target) Run(ctx *Context, client Doer) error {
	var target string
	if len(ctx.Args) > 0 {
		target = ctx.Args[0]
		err := WriteTarget(target)
		if err != nil {
			return err
		}
		io.WriteString(ctx.Stdout, "New target is "+target+"\n")
		return nil
	}
	target = ReadTarget()
	io.WriteString(ctx.Stdout, "Current target is "+target+"\n")
	return nil
}

const DefaultTarget = "http://tsuru.plataformas.glb.com:8080"

func ReadTarget() string {
	targetPath, _ := joinWithUserDir(".tsuru_target")
	if f, err := filesystem().Open(targetPath); err == nil {
		defer f.Close()
		if b, err := ioutil.ReadAll(f); err == nil {
			return string(b)
		}
	}
	return DefaultTarget
}

func GetUrl(path string) string {
	var prefix string
	target := ReadTarget()
	if m, _ := regexp.MatchString("^https?://", target); !m {
		prefix = "http://"
	}
	return prefix + target + path
}

func WriteTarget(t string) error {
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
