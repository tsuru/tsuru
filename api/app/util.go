package app

import (
	"bytes"
	"github.com/timeredbull/tsuru/api/unit"
	"regexp"
)

// filterOutput filters output from juju.
//
// It removes all lines that does not represent useful output, like juju's
// logging and Python's deprecation warnings.
func filterOutput(output []byte, filterFunc func([]byte) bool) []byte {
	var result [][]byte
	var ignore bool
	deprecation := []byte("DeprecationWarning")
	regexLog, err := regexp.Compile(`^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}`)
	if err != nil {
		return output
	}
	regexSshWarning, err := regexp.Compile(`^Warning: Permanently .* to the list of known hosts.$`)
	if err != nil {
		return output
	}
	lines := bytes.Split(output, []byte{'\n'})
	for _, line := range lines {
		if ignore {
			ignore = false
			continue
		}
		if bytes.Contains(line, deprecation) {
			ignore = true
			continue
		}
		if !regexSshWarning.Match(line) &&
			!regexLog.Match(line) &&
			(filterFunc == nil || filterFunc(line)) {
			result = append(result, line)
		}
	}
	return bytes.Join(result, []byte{'\n'})
}

var createRunFileCommand = `cat > /home/application/run-command <<END
#!/bin/bash
[-f /home/application/apprc ] && source /home/application/apprc
[-d /home/application/current] && cd /home/application/current
eval $*
END
`

func createRunFileIfNeed(unit unit.Unit) {
	cmd := "[-f /home/application/run-command] || " + createRunFileCommand + "; chmod +x /home/application/run-command"
	unit.Command(cmd)
}
