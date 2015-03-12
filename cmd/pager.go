// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

type pagerWriter struct {
	baseWriter io.Writer
	pagerPipe  io.WriteCloser
	cmd        *exec.Cmd
	pager      string
}

func (w *pagerWriter) Write(data []byte) (int, error) {
	if w.pagerPipe == nil {
		var err error
		pagerParts := strings.Split(w.pager, " ")
		w.cmd = exec.Command(pagerParts[0], pagerParts[1:]...)
		w.cmd.Stdout = w.baseWriter
		w.pagerPipe, err = w.cmd.StdinPipe()
		if err != nil {
			return 0, err
		}
		err = w.cmd.Start()
		if err != nil {
			return 0, err
		}
	}
	return w.pagerPipe.Write(data)
}

func (w *pagerWriter) close() {
	if w.pagerPipe != nil {
		w.pagerPipe.Close()
		w.cmd.Wait()
	}
}

func newPagerWriter(baseWriter io.Writer) io.Writer {
	pager, found := syscall.Getenv("TSURU_PAGER")
	if found && pager == "" {
		return baseWriter
	}
	outputFile, ok := baseWriter.(*os.File)
	if !ok {
		return baseWriter
	}
	terminalFd := int(outputFile.Fd())
	if !terminal.IsTerminal(terminalFd) {
		return baseWriter
	}
	if pager == "" {
		pager = "less -RFX"
	}
	return &pagerWriter{baseWriter: baseWriter, pager: pager}
}
