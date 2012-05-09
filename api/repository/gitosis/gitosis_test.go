package gitosis

import (
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
)

func (s *S) TestAddProject(c *C) {
	err := AddGroup("someGroup")
	err = AddProject("someGroup", "someProject")
	c.Assert(err, IsNil)
}

func (s *S) TestAddGroup(c *C) {
	err := AddGroup("someGroup")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	//ensures that project have been added to gitosis.conf
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	//ensures that file is not overriden when a new project is added
	err = AddGroup("someOtherGroup")
	c.Assert(err, IsNil)
	// it should have both sections
	conf, err = ini.ReadDefault(path.Join(s.gitRoot, "gitosis-admin/gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	c.Assert(conf.HasSection("group someOtherGroup"), Equals, true)
}

func (s *S) TestAddGroupShouldReturnErrorWhenSectionAlreadyExists(c *C) {
	err := AddGroup("aGroup")
	c.Assert(err, IsNil)
	err = AddGroup("aGroup")
	c.Assert(err, NotNil)
}

func (s *S) TestAddGroupShouldCommitAndPushChangesToGitosisBare(c *C) {
	err := AddGroup("gandalf")
	c.Assert(err, IsNil)
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%H").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisRepo)
	repoOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%H").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	c.Assert(string(repoOutput), Equals, string(bareOutput))
}

func (s *S) TestRemoveGroup(c *C) {
	err := AddGroup("someGroup")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	err = RemoveGroup("someGroup")
	conf, err = ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, false)
	pwd, err := os.Getwd()
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	expected := "Removing group someGroup from gitosis.conf"
	c.Assert(string(bareOutput), Equals, expected)
}

func (s *S) TestRemoveGroupCommitAndPushesChanges(c *C) {
	err := AddGroup("testGroup")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group testGroup"), Equals, true)
	err = RemoveGroup("testGroup")
	conf, err = ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group testGroup"), Equals, false)
}

func (s *S) TestAddMemberToGroup(c *C) {
	err := AddGroup("take-over-the-world")
	c.Assert(err, IsNil)
	err = AddMember("take-over-the-world", "brain")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group take-over-the-world"), Equals, true)
	c.Assert(conf.HasOption("group take-over-the-world", "members"), Equals, true)
	members, err := conf.String("group take-over-the-world", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "brain")
}

func (s *S) TestAddMemberToGroupCommitsAndPush(c *C) {
	err := AddGroup("someTeam")
	c.Assert(err, IsNil)
	err = AddMember("someTeam", "brain")
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	commitMsg := "Adding member brain to group someTeam"
	c.Assert(string(bareOutput), Equals, commitMsg)
}

func (s *S) TestAddTwoMembersToGroup(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "one-of-these-days")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "comfortably-numb")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	members, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "one-of-these-days comfortably-numb")
}

func (s *S) TestAddMemberToGroupReturnsErrorIfTheMemberIsAlreadyInTheGroup(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "time")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "time")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This user is already member of this group$")
}

func (s *S) TestAddMemberToAGroupThatDoesNotExistReturnError(c *C) {
	err := AddMember("pink-floyd", "one-of-these-days")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestRemoveMemberFromGroup(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "summer-68")
	c.Assert(err, IsNil)
	err = RemoveMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	option, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(option, Equals, "summer-68")
}

func (s *S) TestRemoveMemberFromGroupCommitsAndPush(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "atom-heart-mother-suite")
	c.Assert(err, IsNil)
	err = RemoveMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisBare)
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	commitMsg := "Removing member if from group pink-floyd"
	c.Assert(string(bareOutput), Equals, commitMsg)
}

func (s *S) TestRemoveMemberFromGroupRemovesTheOptionFromTheSectionWhenTheMemberIsTheLast(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = AddMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	err = RemoveMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group pink-floyd", "members"), Equals, false)
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorsIfTheGroupDoesNotContainTheGivenMember(c *C) {
	err := AddGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = RemoveMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This group does not have this member$")
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorIfTheGroupDoesNotExist(c *C) {
	err := RemoveMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestAddAndCommit(c *C) {
	confPath := path.Join(s.gitosisRepo, "gitosis.conf")
	conf, err := ini.ReadDefault(confPath)
	c.Assert(err, IsNil)
	conf.AddSection("foo bar")
	pushToGitosis("Some commit message")
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	c.Assert(string(bareOutput), Equals, "Some commit message")
}

func (s *S) TestConfPathReturnsGitosisConfPath(c *C) {
	repoPath, err := config.GetString("git:gitosis-repo")
	expected := path.Join(repoPath, "gitosis.conf")
	obtained, err := ConfPath()
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, expected)
}
