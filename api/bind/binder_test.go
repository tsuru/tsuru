package bind

import (
	"testing"
)

func TestEnvVarStringPrintPublicValue(t *testing.T) {
	env := EnvVar{Name: "PATH", Value: "/", Public: true}
	if env.String() != "PATH=/" {
		t.Errorf("Should print public variable value.\nExpected: PATH=/\nGot: %s", env.String())
	}
}

func TestEnvVarStringMaskPrivateValue(t *testing.T) {
	env := EnvVar{Name: "PATH", Value: "/", Public: false}
	if env.String() != "PATH=*** (private variable)" {
		t.Errorf("Should omit private variable value.\nExpected: PATH=*** (private variable)\nGot: %s", env.String())
	}
}
