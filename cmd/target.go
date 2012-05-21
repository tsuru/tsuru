package cmd

import (
	"errors"
	"io/ioutil"
	"os"
	"syscall"
)

const DefaultTarget = "tsuru.plataformas.glb.com"

func ReadTarget() string {
	targetPath, _ := joinWithUserDir(".tsuru_target")
	if f, err := os.Open(targetPath); err == nil {
		defer f.Close()
		b, _ := ioutil.ReadAll(f)
		return string(b)
	}
	return DefaultTarget
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
	content := []byte(t)
	n, err := targetFile.Write(content)
	if n != len(content) || err != nil {
		return errors.New("Failed to write the target file")
	}
	return nil
}
