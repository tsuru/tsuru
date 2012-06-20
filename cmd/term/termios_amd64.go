// +build !linux

package term

//#include <termios.h>
import "C"

type Termios struct {
	Iflag  uint64
	Oflag  uint64
	Cflag  uint64
	Lflag  uint64
	Cc     [20]uint8
	Ispeed uint64
	Ospeed uint64
}

func tcgetattr(fd uintptr, termios *Termios) {
	var cterm C.struct_termios
	C.tcgetattr(C.int(fd), &cterm)
	var cc [C.NCCS]uint8
	for i, c := range cterm.c_cc {
		cc[i] = uint8(c)
	}
	*termios = Termios{
		Iflag:  uint64(cterm.c_iflag),
		Oflag:  uint64(cterm.c_oflag),
		Cflag:  uint64(cterm.c_cflag),
		Lflag:  uint64(cterm.c_lflag),
		Cc:     cc,
		Ispeed: uint64(cterm.c_ispeed),
		Ospeed: uint64(cterm.c_ospeed),
	}
}
