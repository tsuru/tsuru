// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"fmt"
	"github.com/tsuru/tsuru/cmd/term"
	"log"
	"os"
)

func main() {
	fmt.Print("Enter the password: ")
	password, err := term.ReadPassword(os.Stdin.Fd())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println()
	fmt.Printf("The password is %q.\n", password)
}
