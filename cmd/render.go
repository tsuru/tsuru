// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"

	"github.com/tsuru/tablecli"
)

const (
	pattern  = "\033[%d;%d;%dm%s\033[0m"
	bgFactor = 10
)

var fontColors = map[string]int{
	"black":   30,
	"red":     31,
	"green":   32,
	"yellow":  33,
	"blue":    34,
	"magenta": 35,
	"cyan":    36,
	"white":   37,
}

var fontEffects = map[string]int{
	"reset":   0,
	"bold":    1,
	"inverse": 7,
}

func init() {
	tablecli.TableConfig.BreakOnAny = os.Getenv("TSURU_BREAK_ANY") != ""
	tablecli.TableConfig.ForceWrap = os.Getenv("TSURU_FORCE_WRAP") != ""
	tablecli.TableConfig.UseTabWriter = os.Getenv("TSURU_TAB_WRITER") != ""
}

func Colorfy(msg string, fontcolor string, background string, effect string) string {
	if os.Getenv("TSURU_DISABLE_COLORS") != "" {
		return msg
	}
	return fmt.Sprintf(pattern, fontEffects[effect], fontColors[fontcolor], fontColors[background]+bgFactor, msg)
}
