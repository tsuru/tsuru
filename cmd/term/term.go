package term

import (
	"syscall"
)

//#include <termios.h>
import "C"

func tcsetattr(fd uintptr, when int, termios *Termios) {
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

func GetPassword(fd uintptr) string {
	var termios Termios
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
