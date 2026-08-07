package integration

import (
	"archive/tar"
	"bytes"
	"fmt"
	"net/http"

	check "gopkg.in/check.v1"
)

func uploadFileTest() ExecFlow {
	flow := ExecFlow{
		requires: []string{"team", "poolnames"},
		matrix: map[string]string{
			"pool": "poolnames",
		},
		parallel: true,
	}

	flow.forward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("upload-file-test-%s", env.Get("pool")))
		unitName := "web"

		res := T("app", "create", appName, "go", "-t", "{{.team}}", "-o", "{{.pool}}").Run(env)
		c.Assert(res, ResultOk)

		res = T("unit", "add", "1", "--app", appName, "--process-name", unitName).Run(env)
		c.Assert(res, ResultOk)

		buffer, err := createTestFile()
		c.Assert(err, check.IsNil)

		body := bytes.NewReader(buffer.Bytes())
		url := fmt.Sprintf("/apps/%s/unit/%s/transfer/upload", appName, unitName)

		request, err := http.NewRequest("POST", url, body)
		c.Assert(err, check.IsNil)

		request.Header.Set("Content-Type", "application/x-tar")
		request.Header.Set("x-filepath", "/tmp")

		client := &http.Client{}
		response, err := client.Do(request)
		c.Assert(err, check.IsNil)
		defer response.Body.Close()

		c.Assert(response.StatusCode, check.Equals, http.StatusNoContent)
	}

	flow.backward = func(c *check.C, env *Environment) {
		appName := slugifyName(fmt.Sprintf("mv-swap-autoscale-%s", env.Get("pool")))
		res := T("app", "remove", "-y", "-a", appName).Run(env)
		c.Check(res, ResultOk)
	}

	return flow
}

func createTestFile() (buffer bytes.Buffer, err error) {
	tw := tar.NewWriter(&buffer)
	file := []byte("Hello, world!")
	filename := "transfer.txt"
	fileSize := int64(len(file))
	header := &tar.Header{
		Name: filename,
		Mode: 0600,
		Size: fileSize,
	}

	err = tw.WriteHeader(header)
	if err != nil {
		return
	}
	_, err = tw.Write(file)
	if err != nil {
		return
	}
	err = tw.Close()
	return
}
