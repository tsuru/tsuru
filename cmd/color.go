// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import ("fmt")

const (
    pattern = "\033[%d;%d;%dm%s\033[0m"
    bg_factor = 10
)

var fontcolors = map[string] int {
    "black":   30,
    "red":     31,
    "green":   32,
    "yellow":  33,
    "blue":    34,
    "magenta": 35,
    "cyan":    36,
    "white":   37,
}

var effects = map[string] int {
    "reset":   0,
    "bold":    1,
    "inverse": 7,
}

func colorfy(msg string, fontcolor string, background string, effect string) string{
    return fmt.Sprintf(pattern, effects[effect], fontcolors[fontcolor], fontcolors[background]+bg_factor, msg)
}

func red(msg string) string {
    return colorfy(msg, "red", "", "")
}

func green(msg string) string {
    return colorfy(msg, "green", "", "")
}

func bold_white(msg string) string {
    return colorfy(msg, "white", "", "bold")
}

