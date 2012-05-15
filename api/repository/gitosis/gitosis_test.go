package gitosis

import (
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	. "launchpad.net/gocheck"
	"path"
)

func (s *S) TestaddProject(c *C) {
	err := addGroup("someGroup")
	c.Assert(err, IsNil)
	err = addProject("someGroup", "someProject")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(conf.HasOption("group someGroup", "writable"), Equals, true)
	obtained, err := conf.String("group someGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "someProject")
	// try to add to an inexistent group
	err = addProject("inexistentGroup", "someProject")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Section group inexistentGroup doesn't exists$")
}

func (s *S) TestAddMoreThenOneProject(c *C) {
	err := addGroup("fooGroup")
	c.Assert(err, IsNil)
	err = addProject("fooGroup", "take-over-the-world")
	c.Assert(err, IsNil)
	err = addProject("fooGroup", "someProject")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	obtained, err := conf.String("group fooGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "take-over-the-world someProject")
}

func (s *S) TestRemoveProject(c *C) {
	err := addGroup("fooGroup")
	c.Assert(err, IsNil)
	err = addProject("fooGroup", "fooProject")
	conf, err := getConfig()
	c.Assert(err, IsNil)
	obtained, err := conf.String("group fooGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "fooProject")
	err = removeProject("fooGroup", "fooProject")
	c.Assert(err, IsNil)
	conf, err = getConfig()
	c.Assert(conf.HasOption("group fooGroup", "writable"), Equals, false)
}

func (s *S) TestaddProjectCommitAndPush(c *C) {
	err := addGroup("myGroup")
	c.Assert(err, IsNil)
	err = addProject("myGroup", "myProject")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := "Added project myProject to group myGroup"
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppendToOption(c *C) {
	group := "fooGroup"
	section := fmt.Sprintf("group %s", group)
	err := addGroup(group)
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	err = addOptionValue(conf, section, "writable", "firstProject")
	c.Assert(err, IsNil)
	// Check if option were added
	obtained, err := conf.String(section, "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "firstProject")
	// Add one more value to same section/option
	err = addOptionValue(conf, section, "writable", "anotherProject")
	c.Assert(err, IsNil)
	// Check if the values were appended
	obtained, err = conf.String(section, "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "firstProject anotherProject")
}

func (s *S) TestRemoveOptionValue(c *C) {
	err := addGroup("myGroup")
	c.Assert(err, IsNil)
	err = addProject("myGroup", "myProject")
	c.Assert(err, IsNil)
	err = addProject("myGroup", "myOtherProject")
	c.Assert(err, IsNil)
	// remove one project
	conf, err := ini.Read(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	err = removeOptionValue(conf, "group myGroup", "writable", "myOtherProject")
	c.Assert(err, IsNil)
	obtained, err := conf.String("group myGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "myProject")
	// remove the last project
	err = removeOptionValue(conf, "group myGroup", "writable", "myProject")
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group myGroup", "writable"), Equals, false)
}

func (s *S) TestHasGroup(c *C) {
	err := addGroup("someGroup")
	c.Assert(err, IsNil)
	c.Assert(HasGroup("someGroup"), Equals, true)
	c.Assert(HasGroup("otherGroup"), Equals, false)
}

func (s *S) TestAddGroup(c *C) {
	err := addGroup("someGroup")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	//ensures that project have been added to gitosis.conf
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	//ensures that file is not overriden when a new project is added
	err = addGroup("someOtherGroup")
	c.Assert(err, IsNil)
	// it should have both sections
	conf, err = ini.Read(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	c.Assert(conf.HasSection("group someOtherGroup"), Equals, true)
}

func (s *S) TestAddGroupShouldReturnErrorWhenSectionAlreadyExists(c *C) {
	err := addGroup("aGroup")
	c.Assert(err, IsNil)
	err = addGroup("aGroup")
	c.Assert(err, NotNil)
}

func (s *S) TestAddGroupShouldCommitAndPushChangesToGitosisBare(c *C) {
	err := addGroup("gandalf")
	c.Assert(err, IsNil)
	repoOutput, err := runGit("log", "-1", "--pretty=format:%s")
	c.Assert(err, IsNil)
	c.Assert(repoOutput, Equals, "Defining gitosis group for group gandalf")
}

func (s *S) TestRemoveGroup(c *C) {
	err := addGroup("someGroup")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	err = removeGroup("someGroup")
	conf, err = ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, false)
	got := s.lastBareCommit(c)
	expected := "Removing group someGroup from gitosis.conf"
	c.Assert(got, Equals, expected)
}

func (s *S) TestRemoveGroupCommitAndPushesChanges(c *C) {
	err := addGroup("testGroup")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group testGroup"), Equals, true)
	err = removeGroup("testGroup")
	conf, err = ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group testGroup"), Equals, false)
}

func (s *S) TestAddMemberToGroup(c *C) {
	err := addGroup("take-over-the-world")
	c.Assert(err, IsNil)
	err = addMember("take-over-the-world", "brain")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group take-over-the-world"), Equals, true)
	c.Assert(conf.HasOption("group take-over-the-world", "members"), Equals, true)
	members, err := conf.String("group take-over-the-world", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "brain")
}

func (s *S) TestAddMemberToGroupCommitsAndPush(c *C) {
	err := addGroup("someTeam")
	c.Assert(err, IsNil)
	err = addMember("someTeam", "brain")
	got := s.lastBareCommit(c)
	expected := "Adding member brain to group someTeam"
	c.Assert(got, Equals, expected)
}

func (s *S) TestAddTwoMembersToGroup(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "one-of-these-days")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "comfortably-numb")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	members, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "one-of-these-days comfortably-numb")
}

func (s *S) TestAddMemberToGroupReturnsErrorIfTheMemberIsAlreadyInTheGroup(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "time")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "time")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Value time for option members in section group pink-floyd has already been added$")
}

func (s *S) TestAddMemberToAGroupThatDoesNotExistReturnError(c *C) {
	err := addMember("pink-floyd", "one-of-these-days")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestRemoveMemberFromGroup(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "summer-68")
	c.Assert(err, IsNil)
	err = removeMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	option, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(option, Equals, "summer-68")
}

func (s *S) TestRemoveMemberFromGroupCommitsAndPush(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "atom-heart-mother-suite")
	c.Assert(err, IsNil)
	err = removeMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := "Removing member if from group pink-floyd"
	c.Assert(got, Equals, expected)
}

func (s *S) TestRemoveMemberFromGroupRemovesTheOptionFromTheSectionWhenTheMemberIsTheLast(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	err = removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group pink-floyd", "members"), Equals, false)
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorsIfTheGroupDoesNotContainTheGivenMember(c *C) {
	err := addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = addMember("pink-floyd", "another-brick")
	c.Assert(err, IsNil)
	err = removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Value pigs-on-the-wing not found in section group pink-floyd$")
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorIfTheGroupDoesNotExist(c *C) {
	err := removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorsIfTheGroupDoesNotHaveAnyMember(c *C) {
	err := addGroup("pato-fu")
	c.Assert(err, IsNil)
	err = removeMember("pato-fu", "eu-sei")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This group does not have any members$")
}

func (s *S) TestAddAndCommit(c *C) {
	confPath := path.Join(s.gitosisRepo, "gitosis.conf")
	conf, err := ini.Read(confPath, ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	conf.AddSection("foo bar")
	pushToGitosis("Some commit message")
	got := s.lastBareCommit(c)
	c.Assert(got, Equals, "Some commit message")
}

func (s *S) TestConfPathReturnsGitosisConfPath(c *C) {
	repoPath, err := config.GetString("git:gitosis-repo")
	expected := path.Join(repoPath, "gitosis.conf")
	obtained, err := ConfPath()
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, expected)
}
