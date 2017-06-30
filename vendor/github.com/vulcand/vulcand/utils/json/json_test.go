package json

import (
	"bytes"
	"testing"
)

func TestMarshal(t *testing.T) {
	for _, tCase := range []struct {
		in  interface{}
		out []byte
	}{
		{map[string]string{}, []byte("{}")},
		{map[string]string{"fiz": "f&z"}, []byte(`{"fiz":"f&z"}`)},
		{"foo & bar", []byte(`"foo & bar"`)},
	} {
		out, err := Marshal(tCase.in)

		if err != nil {
			t.Errorf("Unexecpted error: %s", err.Error())
		}

		if !bytes.Equal(out, append(tCase.out, []byte("\n")...)) {
			t.Errorf(
				"Wrong result: [%+v] instead of [%+v]",
				string(out),
				string(tCase.out),
			)
		}
	}
}
