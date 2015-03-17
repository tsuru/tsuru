// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

type descriptable interface {
	Fd() uintptr
}

type pagerWriter struct {
	baseWriter io.Writer
	pagerPipe  io.WriteCloser
	cmd        *exec.Cmd
	pager      string
	buf        bytes.Buffer
	height     int
	erroed     bool
}

func (w *pagerWriter) Write(data []byte) (int, error) {
	if w.pagerPipe != nil {
		return w.pagerPipe.Write(data)
	}
	if w.erroed {
		return w.baseWriter.Write(data)
	}
	w.buf.Write(data)
	lines := bytes.Count(w.buf.Bytes(), []byte{'\n'})
	if lines >= w.height {
		if w.pagerPipe == nil {
			var err error
			pagerParts := strings.Split(w.pager, " ")
			w.cmd = exec.Command(pagerParts[0], pagerParts[1:]...)
			w.cmd.Stdout = w.baseWriter
			w.pagerPipe, err = w.cmd.StdinPipe()
			if err != nil {
				w.erroed = true
			}
			err = w.cmd.Start()
			if err != nil {
				w.pagerPipe = nil
				w.erroed = true
			}
		}
		w.flush()
	}
	return len(data), nil
}

func (w *pagerWriter) flush() {
	if w.pagerPipe != nil {
		w.pagerPipe.Write(w.buf.Bytes())
	} else {
		w.baseWriter.Write(w.buf.Bytes())
	}
	w.buf.Reset()
}

func (w *pagerWriter) close() {
	w.flush()
	if w.pagerPipe != nil {
		w.pagerPipe.Close()
		w.cmd.Wait()
		w.pagerPipe = nil
		w.cmd = nil
	}
}

type syncReader struct {
	baseReader io.Reader
	pager      *pagerWriter
}

func (r *syncReader) Read(p []byte) (int, error) {
	if r.pager != nil {
		r.pager.close()
	}
	return r.baseReader.Read(p)
}

func (r *syncReader) Fd() uintptr {
	if r.pager != nil {
		r.pager.close()
	}
	if desc, ok := r.baseReader.(descriptable); ok {
		return desc.Fd()
	}
	return 0
}

func newSyncReader(baseReader io.Reader, writerToSync io.Writer) io.Reader {
	pager, _ := writerToSync.(*pagerWriter)
	return &syncReader{pager: pager, baseReader: baseReader}
}

func newPagerWriter(baseWriter io.Writer) io.Writer {
	pager, found := syscall.Getenv("TSURU_PAGER")
	if found && pager == "" {
		return baseWriter
	}
	outputDesc, ok := baseWriter.(descriptable)
	if !ok {
		return baseWriter
	}
	terminalFd := int(outputDesc.Fd())
	if !terminal.IsTerminal(terminalFd) {
		return baseWriter
	}
	_, ttyHeight, _ := terminal.GetSize(terminalFd)
	if pager == "" {
		pager = "less -RFX"
	}
	return &pagerWriter{baseWriter: baseWriter, pager: pager, height: ttyHeight}
}
