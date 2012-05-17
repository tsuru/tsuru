package gitosis

import (
	"errors"
	. "launchpad.net/gocheck"
)

func (s *S) TestDoneDoesNothingIfTheChannelIsNil(c *C) {
	defer func() {
		if r := recover(); r != nil {
			c.Errorf("should not fail")
		}
	}()
	a := newAgent(nil)
	a.done(nil, nil)
}

func (s *S) TestDoneReturnsSuccessIfTheChannelIsNotNilButErrorIs(c *C) {
	a := newAgent(nil)
	ch := make(chan string)
	go a.done(ch, nil)
	result := <-ch
	c.Assert(result, Equals, "success")
}

func (s *S) TestDoneReturnFailAndTheErrorMessageIfNeitherTheChannelNorTheErrorIsNil(c *C) {
	a := newAgent(nil)
	ch := make(chan string)
	go a.done(ch, errors.New("I have failed"))
	result := <-ch
	c.Assert(result, Equals, "fail: I have failed")
}

func (s *S) TestShouldHaveConstantForAddKey(c *C) {
	c.Assert(AddKey, Equals, 0)
}

func (s *S) TestAddKeyReturnsTheKeyFileNameInTheResponseChannel(c *C) {
	a := newAgent(newRecordingManager())
	go a.loop()
	response := make(chan string)
	change := Change{
		Kind: AddKey,
		Args: map[string]string{
			"key":    "so-pure",
			"member": "alanis-morissette",
		},
		Response: response,
	}
	a.Process(change)
	k := <-response
	c.Assert(k, Equals, "alanis-morissette_key1.pub")
}

func (s *S) TestShouldHaveConstantForRemoveKey(c *C) {
	c.Assert(RemoveKey, Equals, 1)
}

func (s *S) TestRemoveKeyChangeRemovesTheKey(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     RemoveKey,
		Args:     map[string]string{"key": "mykey.pub"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["removeKey"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"mykey.pub"})
}

func (s *S) TestShouldHaveConstantForAddMember(c *C) {
	c.Assert(AddMember, Equals, 2)
}

func (s *S) TestAddMemberChangeAddsTheMemberToTheFile(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     AddMember,
		Args:     map[string]string{"group": "dream-theater", "member": "octavarium"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["addMember"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"dream-theater", "octavarium"})
}

func (s *S) TestShouldHaveConstantForRemoveMember(c *C) {
	c.Assert(RemoveMember, Equals, 3)
}

func (s *S) TestRemoveMemberChangeRemovesTheMemberFromTheFile(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     RemoveMember,
		Args:     map[string]string{"group": "dream-theater", "member": "the-glass-prision"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["removeMember"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"dream-theater", "the-glass-prision"})
}

func (s *S) TestShouldHaveConstantForAddGroup(c *C) {
	c.Assert(AddGroup, Equals, 4)
}

func (s *S) TestAddGroupChangeAddsAGroupToGitosisConf(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     AddGroup,
		Args:     map[string]string{"group": "dream-theater"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["addGroup"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"dream-theater"})
}

func (s *S) TestShouldHaveConstantForRemoveGroup(c *C) {
	c.Assert(RemoveGroup, Equals, 5)
}

func (s *S) TestRemoveGroupChangeRemovesTheGroupFromGitosisConf(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     RemoveGroup,
		Args:     map[string]string{"group": "steve-lee"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["removeGroup"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"steve-lee"})
}

func (s *S) TestShouldHaveConstantForAddProject(c *C) {
	c.Assert(AddProject, Equals, 6)
}

func (s *S) TestAddProjectChangeAddsAProjectToTheGroup(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     AddProject,
		Args:     map[string]string{"group": "rush", "project": "grace-under-pressure"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["addProject"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"rush", "grace-under-pressure"})
}

func (s *S) TestShouldHaveContantForRemoveProject(c *C) {
	c.Assert(RemoveProject, Equals, 7)
}

func (s *S) TestRemoveProjectChangeRemovesAProjectFromTheGroup(c *C) {
	r := newRecordingManager()
	a := newAgent(r)
	go a.loop()
	change := Change{
		Kind:     RemoveProject,
		Args:     map[string]string{"group": "nando-reis", "project": "ao-vivo"},
		Response: make(chan string),
	}
	a.Process(change)
	<-change.Response
	action, ok := r.actions["removeProject"]
	c.Assert(ok, Equals, true)
	c.Assert(action, DeepEquals, []string{"nando-reis", "ao-vivo"})
}
