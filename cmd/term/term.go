// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package term

import (
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

func ReadPassword(fd uintptr) (string, error) {
	var termios, oldState syscall.Termios
	if _, _, e := syscall.Syscall6(syscall.SYS_IOCTL, fd, TCGETS,
		uintptr(unsafe.Pointer(&termios)), 0, 0, 0); e == 0 {
		oldState = termios
		termios.Lflag &^= syscall.ECHO
		if _, _, e := syscall.Syscall6(syscall.SYS_IOCTL, fd, TCSETS,
			uintptr(unsafe.Pointer(&termios)), 0, 0, 0); e == 0 {
		}
		// Restoring after reading the password
		defer syscall.Syscall6(syscall.SYS_IOCTL, fd, TCSETS, uintptr(unsafe.Pointer(&oldState)), 0, 0, 0)
		// Restoring on SIGINT
		sigChan := make(chan os.Signal, 1)
		go func(c chan os.Signal, t syscall.Termios, fd uintptr) {
			if _, ok := <-c; ok {
				syscall.Syscall6(syscall.SYS_IOCTL, fd, TCSETS, uintptr(unsafe.Pointer(&oldState)), 0, 0, 0)
				os.Exit(1)
			}
		}(sigChan, oldState, fd)
		defer close(sigChan)
		signal.Notify(sigChan, syscall.SIGINT)
	}
	var buf [16]byte
	var pass []byte
	for {
		n, err := syscall.Read(int(fd), buf[:])
		if n == 0 {
			break
		}
		if err != nil {
			return "", err
		}
		for n > 0 && (buf[n-1] == '\n' || buf[n-1] == '\r') {
			n--
		}
		pass = append(pass, buf[:n]...)
		if n < len(buf) {
			break
		}
	}
	return string(pass), nil
}
