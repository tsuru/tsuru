// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the http://golang.org/LICENSE file.

// This code is inspired by integration tests present in the package
// code.google.com/p/go.crypto/ssh, with some changes by the tsuru authors.

package docker

import (
	"bytes"
	"io/ioutil"
	"launchpad.net/gocheck"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"text/template"
	"time"
)

const sshd_config = `
Protocol 2
HostKey {{.Dir}}/id_rsa
Pidfile {{.Dir}}/sshd.pid
Port {{.Port}}
KeyRegenerationInterval 3600
ServerKeyBits 768
SyslogFacility AUTH
LogLevel DEBUG2
LoginGraceTime 120
PermitRootLogin no
StrictModes no
RSAAuthentication yes
PubkeyAuthentication yes
AuthorizedKeysFile	{{.Dir}}/id_rsa.pub
IgnoreRhosts yes
RhostsRSAAuthentication no
HostbasedAuthentication no
`

var configTmpl = template.Must(template.New("").Parse(sshd_config))

type sshServer struct {
	c          *gocheck.C
	cleanup    func() // executed during Shutdown
	configfile string
	cmd        *exec.Cmd
	output     bytes.Buffer // holds stderr from sshd process
	port       string
}

func sshUsername() string {
	var username string
	if user, err := user.Current(); err == nil {
		username = user.Username
	} else {
		log.Printf("user.Current: %v; falling back on $USER", err)
		username = os.Getenv("USER")
	}
	if username == "" {
		panic("Unable to get username")
	}
	return username
}

func (s *sshServer) start() {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		s.c.Skip("skipping test: " + err.Error())
	}
	s.cmd = exec.Command(sshd, "-f", s.configfile, "-e", "-D")
	s.cmd.Stdout = &s.output
	s.cmd.Stderr = &s.output
	if err := s.cmd.Start(); err != nil {
		s.c.Fail()
		s.Shutdown()
		s.c.Fatalf("s.cmd.Start: %v", err)
	}
}

func (s *sshServer) Shutdown() {
	if s.cmd != nil && s.cmd.Process != nil {
		// Don't check for errors; if it fails it's most
		// likely "os: process already finished", and we don't
		// care about that. Use os.Interrupt, so child
		// processes are killed too.
		s.cmd.Process.Signal(os.Interrupt)
		s.cmd.Wait()
	}
	if s.c.Failed() {
		// log any output from sshd process
		s.c.Log("sshd: " + s.output.String())
	}
	s.cleanup()
}

func writeFile(path string, contents []byte) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := f.Write(contents); err != nil {
		panic(err)
	}
}

func getAvailablePort() string {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return port
}

// newMockSSHServer returns a new mock ssh server.
func newMockSSHServer(c *gocheck.C, timeout time.Duration) *sshServer {
	dir, err := ioutil.TempDir("", "sshtest")
	c.Assert(err, gocheck.IsNil)
	f, err := os.Create(filepath.Join(dir, "sshd_config"))
	c.Assert(err, gocheck.IsNil)
	port := getAvailablePort()
	err = configTmpl.Execute(f, map[string]string{
		"Dir":  dir,
		"Port": port,
	})
	c.Assert(err, gocheck.IsNil)
	f.Close()

	ioutil.WriteFile(filepath.Join(dir, "id_rsa"), fakeServerPrivateKey, 0600)
	ioutil.WriteFile(filepath.Join(dir, "id_rsa.pub"), fakeServerPublicKey, 0644)

	server := sshServer{
		c:          c,
		configfile: f.Name(),
		cleanup: func() {
			err := os.RemoveAll(dir)
			c.Assert(err, gocheck.IsNil)
		},
		port: port,
	}
	server.start()
	timedout := make(chan bool)
	quit := make(chan bool)
	go func() {
		addr := "localhost:" + port
		for {
			select {
			case <-timedout:
				return
			default:
				if conn, err := net.Dial("tcp", addr); err == nil {
					conn.Close()
					close(quit)
					return
				}
			}
		}
	}()
	select {
	case <-quit:
	case <-time.After(timeout):
		close(timedout)
		c.Fatalf("The SSH server didn't come up after %s.", timeout)
	}
	return &server
}

var fakeServerPrivateKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQCj9Zd3Vhrq4GbZ3Ed8HcBJBcW7GVdVUDRmu7vTbIJ9B435QKG7
CpLAL8SHULHETDsKZliuaL+JZgTxArGKycEOCBW30NsURnTBgOuURFkkR+4++Hhx
+VCR5+Z9Gu5BPZTNdGRU8z1C4+GCgIU7FdVJK+Qj00WKBMcTbb89/6z15wIDAQAB
AoGATQhLFKdg2C98QylqcJbty6EpqGEclhmrtQTJF2lo2WNeQdgq5FzwW9lVhZnV
G3wRVS6GxdKzAtPqyG1SivmFeNh2uj+tohxhNRQsDKSt1K4it3UctfGOeZU8pIp2
iheYFej0boKhf1Llk5OwTXGlMfD0nkpdD0kMUSjIpO/q4BECQQDXLOFw9uqcr+Ek
BT8ge1lGGlCyChKT1pMkU9xivSL7s36AzG93pus0jNAYd3gOftP+IKrIUypfH/XF
whhpmGhvAkEAwxEjaOm+RyTzV0L+tXNxsnmAYWrY1IZVCJx/nsAAAEv/0ULyfEs9
0P7bpo1Ov3LUNTd4Jz7AAb5G+f4dE4RWCQJAWDR0oasGF37dird/3h/SQ7Nr2t/Y
J7QxExYxZGRl38n/lGq5UtIg3qTOdQkcNMz2t9jKSV4WI3JlfFCJU1f/jwJBALRY
cQN7L7d4+x3PS8wYqqKWYNIwRb3fYFiwz/DGlHmxyhb/rU6rBcDnD86hUJACKx30
Zbrq8fvqnpZckSdNL3kCQQDNbVWjqR7d472Ble/dWvohFTfZQ2pEWym8Ars29Sg1
vznme2VX7myWVmDiMK4Dy4VuJWgcqZpZm0dGNAf0f2kk
-----END RSA PRIVATE KEY-----
`)

var fakeServerPublicKey = []byte("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCj9Zd3Vhrq4GbZ3Ed8HcBJBcW7GVdVUDRmu7vTbIJ9B435QKG7CpLAL8SHULHETDsKZliuaL+JZgTxArGKycEOCBW30NsURnTBgOuURFkkR+4++Hhx+VCR5+Z9Gu5BPZTNdGRU8z1C4+GCgIU7FdVJK+Qj00WKBMcTbb89/6z15w== f@xikinbook.local\n")
