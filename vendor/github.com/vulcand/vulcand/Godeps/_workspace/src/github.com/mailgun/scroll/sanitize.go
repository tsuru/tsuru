package scroll

import (
	"fmt"
	"unicode"
)

// The AllowSet interface is implemented to detect if input is safe or not.
type AllowSet interface {
	IsSafe(string) error
}

// AllowSetBytes allows the definition of a set of safe allowed ASCII characters.
// AllowSetBytes does not support unicode code points. If you pass the
// string "ü" (which encodes as 0xc3 0xbc) they will be skipped over.
type AllowSetBytes struct {
	maxLen int
	chars  [256]bool
}

func NewAllowSetBytes(s string, maxlen int) AllowSetBytes {
	var as [256]bool
	for i := 0; i < len(s); i++ {
		if s[i] <= unicode.MaxASCII {
			as[s[i]] = true
		}
	}
	return AllowSetBytes{maxLen: maxlen, chars: as}
}

func (a AllowSetBytes) IsSafe(s string) error {
	if len(s) > a.maxLen {
		return fmt.Errorf("length %v, longer then maximum allowable length: %v", len(s), a.maxLen)
	}

	for i := 0; i < len(s); i++ {
		if a.chars[s[i]] == false {
			return fmt.Errorf("character %q (%v) not allowed", string(s[i]), s[i])
		}
	}

	return nil
}

// AllowSetStrings allows the definition of a set of safe allowed strings.
type AllowSetStrings struct {
	strings map[string]bool
}

func NewAllowSetStrings(s []string) AllowSetStrings {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return AllowSetStrings{strings: m}
}

func (a AllowSetStrings) IsSafe(s string) error {
	if _, ok := a.strings[s]; !ok {
		return fmt.Errorf("string %v not allowed", s)
	}
	return nil
}
