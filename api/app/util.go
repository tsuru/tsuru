package app

import (
	"bytes"
	"regexp"
)

// filterOutput filters output from juju.
//
// It removes all lines that does not represent useful output, like juju's
// logging and Python's deprecation warnings.
func filterOutput(output []byte) []byte {
	var result []byte
	var ignore bool
	deprecation := []byte("DeprecationWarning")
	regexLog, err := regexp.Compile(`^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}`)
	if err != nil {
		return output
	}
	buf := bytes.NewBuffer(output)
	for line, err := buf.ReadBytes('\n'); err == nil || len(line) > 0; line, err = buf.ReadBytes('\n') {
		if ignore {
			ignore = false
			continue
		}
		if bytes.Contains(line, deprecation) {
			ignore = true
			continue
		}
		if !regexLog.Match(line) {
			result = append(result, line...)
		}
	}
	return result
}
