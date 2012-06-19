package cmd

//#include <termios.h>
import "C"

import (
	"syscall"
)

func tcsetattr(fd uintptr, when int, termios *syscall.Termios) {
	var cterm C.struct_termios
	var cc_t [C.NCCS]C.cc_t
	for i, c := range termios.Cc {
		cc_t[i] = C.cc_t(c)
	}
	cterm.c_iflag = C.tcflag_t(termios.Iflag)
	cterm.c_oflag = C.tcflag_t(termios.Oflag)
	cterm.c_cflag = C.tcflag_t(termios.Cflag)
	cterm.c_lflag = C.tcflag_t(termios.Lflag)
	cterm.c_cc = cc_t
	cterm.c_ispeed = C.speed_t(termios.Ispeed)
	cterm.c_ospeed = C.speed_t(termios.Ospeed)
	C.tcsetattr(C.int(fd), C.int(when), &cterm)
}

func tcgetattr(fd uintptr, termios *syscall.Termios) {
	var cterm C.struct_termios
	C.tcgetattr(C.int(fd), &cterm)
	var cc [C.NCCS]uint8
	for i, c := range cterm.c_cc {
		cc[i] = uint8(c)
	}
	*termios = syscall.Termios{
		Iflag:  uint64(cterm.c_iflag),
		Oflag:  uint64(cterm.c_oflag),
		Cflag:  uint64(cterm.c_cflag),
		Lflag:  uint64(cterm.c_lflag),
		Cc:     cc,
		Ispeed: uint64(cterm.c_ispeed),
		Ospeed: uint64(cterm.c_ospeed),
	}
}

func getPassword(fd uintptr) string {
	var termios syscall.Termios
	tcgetattr(fd, &termios)
	termios.Lflag &^= syscall.ECHO
	tcsetattr(fd, 0, &termios)
	var buf [16]byte
	var pass []byte
	for {
		n, _ := syscall.Read(int(fd), buf[:])
		if n == 0 {
			break
		}
		if buf[n-1] == '\n' {
			n--
		}
		pass = append(pass, buf[:n]...)
		if n < len(buf) {
			break
		}
	}
	password := string(pass)
	var last uint8
	if password != "" {
		for last = password[len(password)-1]; last == '\r' || last == '\n'; last = password[len(password)-1] {
			password = password[:len(password)-1]
		}
	}
	return password
}
