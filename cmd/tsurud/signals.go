// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/tsuru/config"
)

func listenSignals() {
	ch := make(chan os.Signal, 2)
	go func() {
		for sig := range ch {
			switch sig {
			case syscall.SIGUSR1:
				pprof.Lookup("goroutine").WriteTo(os.Stdout, 2)
			case syscall.SIGHUP:
				config.ReadConfigFile(configPath)
			}
		}
	}()
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGUSR1)
}
