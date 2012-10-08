// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

const (
	AddKey = iota
	RemoveKey
	AddMember
	RemoveMember
	AddGroup
	RemoveGroup
	AddProject
	RemoveProject
	Commit
)

// Change encapsulates a change that will be requested to the gitosis file.
//
// The kind is an integer, but you should not send a magic number. Try sending
// one of the package's constant, and Args represent the args for the kind of
// change. If the change provide any response, it will be sent though the
// response channel (a string channel). Example:
//
//     args := map[string]string{
//         "key":    "my-key",
//         "member": "gopher",
//     }
//     change := Change{Kind: AddKey, Args: args}
//
// The change in the code above says:
//
//     "add the key my-key to the member gopher"
//
// For this kind of change, the key file name will be sent in the channel
// Response.
type Change struct {
	Kind     int
	Args     map[string]string
	Response chan string
}

// Ag is the Agent instance that should be used to send changes to gitosis repository.
//
// Sample of code:
//
//     import "github.com/globocom/tsuru/repository"
//
//     func sendChange() {
//         change := repository.Change{
//             Kind:     repository.AddKey,
//             Args:     map[string]string{"key": "the-key", "member": "gopher"},
//             Response: make(chan string),
//         }
//         repository.Ag.Process(change)
//         resp := <-change.Response
//         // do something with the response
//     }
var Ag *Agent

// RunAgent starts the agent loop, so it keep looking for changes to be
// processed.
func RunAgent() {
	gitosisManager, err := newGitosisManager()
	if err != nil {
		panic(err)
	}
	Ag = newAgent(gitosisManager)
	go Ag.loop()
}

// Agent listens for changes and process them.
type Agent struct {
	changes chan Change
	mngr    manager
}

// newAgent returns an instance of a new Agent.
func newAgent(m manager) *Agent {
	return &Agent{
		changes: make(chan Change),
		mngr:    m,
	}
}

// Process sends a change to the agent queue.
func (a *Agent) Process(change Change) {
	a.changes <- change
}

// loop loops "forever" processing changes that come in the agent queue.
func (a *Agent) loop() {
	for change := range a.changes {
		switch change.Kind {
		case AddKey:
			go func(ch Change) {
				keyfile, _ := a.mngr.buildAndStoreKeyFile(change.Args["member"], change.Args["key"])
				ch.Response <- keyfile
			}(change)
		case RemoveKey:
			go func(ch Change) {
				err := a.mngr.deleteKeyFile(ch.Args["key"])
				a.done(ch.Response, err)
			}(change)
		case AddMember:
			go func(ch Change) {
				err := a.mngr.addMember(ch.Args["group"], ch.Args["member"])
				a.done(ch.Response, err)
			}(change)
		case RemoveMember:
			go func(ch Change) {
				err := a.mngr.removeMember(ch.Args["group"], ch.Args["member"])
				a.done(ch.Response, err)
			}(change)
		case AddGroup:
			go func(ch Change) {
				err := a.mngr.addGroup(ch.Args["group"])
				a.done(ch.Response, err)
			}(change)
		case RemoveGroup:
			go func(ch Change) {
				err := a.mngr.removeGroup(ch.Args["group"])
				a.done(ch.Response, err)
			}(change)
		case AddProject:
			go func(ch Change) {
				err := a.mngr.addProject(ch.Args["group"], ch.Args["project"])
				a.done(ch.Response, err)
			}(change)
		case RemoveProject:
			go func(ch Change) {
				err := a.mngr.removeProject(ch.Args["group"], ch.Args["project"])
				a.done(ch.Response, err)
			}(change)
		case Commit:
			go func(ch Change) {
				err := a.mngr.commit(ch.Args["message"])
				a.done(ch.Response, err)
			}(change)
		}
	}
}

func (a *Agent) done(ch chan string, err error) {
	if ch != nil {
		if err != nil {
			ch <- "fail: " + err.Error()
		} else {
			ch <- "success"
		}
	}
}
