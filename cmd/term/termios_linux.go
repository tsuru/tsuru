// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package term

//#include <termios.h>
import "C"

import (
	"syscall"
)

type Termios syscall.Termios

func tcgetattr(fd uintptr, termios *Termios) {
	var cterm C.struct_termios
	C.tcgetattr(C.int(fd), &cterm)
	var cc [C.NCCS]uint8
	for i, c := range cterm.c_cc {
		cc[i] = uint8(c)
	}
	*termios = Termios{
		Iflag:  uint32(cterm.c_iflag),
		Oflag:  uint32(cterm.c_oflag),
		Cflag:  uint32(cterm.c_cflag),
		Lflag:  uint32(cterm.c_lflag),
		Cc:     cc,
		Ispeed: uint32(cterm.c_ispeed),
		Ospeed: uint32(cterm.c_ospeed),
	}
}
