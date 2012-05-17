package gitosis

import (
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"syscall"
)

func (s *S) TestNewGitosisManager(c *C) {
	p, err := config.GetString("git:gitosis-repo")
	c.Assert(err, IsNil)
	expectedPath := path.Join(p, "gitosis.conf")
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	c.Assert(m.confPath, Equals, expectedPath)
}

func (s *S) TestGitosisManagerShoulImplementManager(c *C) {
	var iManager manager
	gitosisManager, _ := newGitosisManager()
	c.Assert(gitosisManager, Implements, &iManager)
}

func (s *S) TestAddProject(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("someGroup")
	c.Assert(err, IsNil)
	err = m.addProject("someGroup", "someProject")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group someGroup", "writable"), Equals, true)
	obtained, err := conf.String("group someGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "someProject")
	// try to add to an inexistent group
	err = m.addProject("inexistentGroup", "someProject")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Section group inexistentGroup doesn't exists$")
}

func (s *S) TestAddMoreThenOneProject(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("fooGroup")
	c.Assert(err, IsNil)
	err = m.addProject("fooGroup", "take-over-the-world")
	c.Assert(err, IsNil)
	err = m.addProject("fooGroup", "someProject")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	obtained, err := conf.String("group fooGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "take-over-the-world someProject")
}

func (s *S) TestRemoveProject(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("fooGroup")
	c.Assert(err, IsNil)
	err = m.addProject("fooGroup", "fooProject")
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	obtained, err := conf.String("group fooGroup", "writable")
	c.Assert(err, IsNil)
	c.Assert(obtained, Equals, "fooProject")
	err = m.removeProject("fooGroup", "fooProject")
	c.Assert(err, IsNil)
	conf, err = m.getConfig()
	c.Assert(conf.HasOption("group fooGroup", "writable"), Equals, false)
}

func (s *S) TestRemoveProjectReturnsErrorIfTheGroupDoesNotExist(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.removeProject("nando-reis", "ao-vivo")
	c.Assert(err, NotNil)
}

func (s *S) TestRemoveProjectReturnsErrorIfTheProjectDoesNotExist(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("nando-reis")
	c.Assert(err, IsNil)
	err = m.removeProject("nando-reis", "ao-vivo")
	c.Assert(err, NotNil)
}

func (s *S) TestRemoveProjectCommitsWithProperMessage(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("nando-reis")
	c.Assert(err, IsNil)
	err = m.addProject("nando-reis", "ao-vivo")
	c.Assert(err, IsNil)
	err = m.removeProject("nando-reis", "ao-vivo")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := "Removing project ao-vivo from group nando-reis"
	c.Assert(got, Equals, expected)
}

func (s *S) TestAddProjectCommitAndPush(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("myGroup")
	c.Assert(err, IsNil)
	err = m.addProject("myGroup", "myProject")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := "Added project myProject to group myGroup"
	c.Assert(got, Equals, expected)
}

func (s *S) TestAppendToOption(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	group := "fooGroup"
	section := fmt.Sprintf("group %s", group)
	err = m.addGroup(group)
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
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
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("myGroup")
	c.Assert(err, IsNil)
	err = m.addProject("myGroup", "myProject")
	c.Assert(err, IsNil)
	err = m.addProject("myGroup", "myOtherProject")
	c.Assert(err, IsNil)
	// remove one project
	conf, err := m.getConfig()
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
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("someGroup")
	c.Assert(err, IsNil)
	c.Assert(m.hasGroup("someGroup"), Equals, true)
	c.Assert(m.hasGroup("otherGroup"), Equals, false)
}

func (s *S) TestAddGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("someGroup")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	//ensures that project have been added to gitosis.conf
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	//ensures that file is not overriden when a new project is added
	err = m.addGroup("someOtherGroup")
	c.Assert(err, IsNil)
	// it should have both sections
	conf, err = m.getConfig()
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, true)
	c.Assert(conf.HasSection("group someOtherGroup"), Equals, true)
}

func (s *S) TestAddGroupShouldReturnErrorWhenSectionAlreadyExists(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("aGroup")
	c.Assert(err, IsNil)
	err = m.addGroup("aGroup")
	c.Assert(err, NotNil)
}

func (s *S) TestAddGroupShouldCommitAndPushChangesToGitosisBare(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("gandalf")
	c.Assert(err, IsNil)
	repoOutput, err := s.mngr.git.run("log", "-1", "--pretty=format:%s")
	c.Assert(err, IsNil)
	c.Assert(repoOutput, Equals, "Defining gitosis group for group gandalf")
}

func (s *S) TestRemoveGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("someGroup")
	c.Assert(err, IsNil)
	err = m.removeGroup("someGroup")
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group someGroup"), Equals, false)
	got := s.lastBareCommit(c)
	expected := "Removing group someGroup from gitosis.conf"
	c.Assert(got, Equals, expected)
}

func (s *S) TestRemoveGroupCommitAndPushesChanges(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("testGroup")
	c.Assert(err, IsNil)
	err = m.removeGroup("testGroup")
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	c.Assert(conf.HasSection("group testGroup"), Equals, false)
}

func (s *S) TestAddMemberToGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("take-over-the-world")
	c.Assert(err, IsNil)
	err = m.addMember("take-over-the-world", "brain")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group take-over-the-world", "members"), Equals, true)
	members, err := conf.String("group take-over-the-world", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "brain")
}

func (s *S) TestAddMemberToGroupCommitsAndPush(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("someTeam")
	c.Assert(err, IsNil)
	err = m.addMember("someTeam", "brain")
	got := s.lastBareCommit(c)
	expected := "Adding member brain to group someTeam"
	c.Assert(got, Equals, expected)
}

func (s *S) TestAddTwoMembersToGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "one-of-these-days")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "comfortably-numb")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	members, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "one-of-these-days comfortably-numb")
}

func (s *S) TestAddMemberToGroupReturnsErrorIfTheMemberIsAlreadyInTheGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "time")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "time")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Value time for option members in section group pink-floyd has already been added$")
}

func (s *S) TestAddMemberToAGroupThatDoesNotExistReturnError(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "one-of-these-days")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestRemoveMemberFromGroup(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "summer-68")
	c.Assert(err, IsNil)
	err = m.removeMember("pink-floyd", "fat-old-sun")
	c.Assert(err, IsNil)
	conf, err := m.getConfig()
	c.Assert(err, IsNil)
	option, err := conf.String("group pink-floyd", "members")
	c.Assert(err, IsNil)
	c.Assert(option, Equals, "summer-68")
}

func (s *S) TestRemoveMemberFromGroupCommitsAndPush(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "atom-heart-mother-suite")
	c.Assert(err, IsNil)
	err = m.removeMember("pink-floyd", "if")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := "Removing member if from group pink-floyd"
	c.Assert(got, Equals, expected)
}

func (s *S) TestRemoveMemberFromGroupRemovesTheOptionFromTheSectionWhenTheMemberIsTheLast(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	err = m.removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, IsNil)
	conf, err := ini.Read(path.Join(s.gitosisRepo, "gitosis.conf"), ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	c.Assert(conf.HasOption("group pink-floyd", "members"), Equals, false)
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorsIfTheGroupDoesNotContainTheGivenMember(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pink-floyd")
	c.Assert(err, IsNil)
	err = m.addMember("pink-floyd", "another-brick")
	c.Assert(err, IsNil)
	err = m.removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Value pigs-on-the-wing not found in section group pink-floyd$")
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorIfTheGroupDoesNotExist(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.removeMember("pink-floyd", "pigs-on-the-wing")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestRemoveMemberFromGroupReturnsErrorsIfTheGroupDoesNotHaveAnyMember(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.addGroup("pato-fu")
	c.Assert(err, IsNil)
	err = m.removeMember("pato-fu", "eu-sei")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^This group does not have any members$")
}

func (s *S) TestAddAndCommit(c *C) {
	confPath := path.Join(s.gitosisRepo, "gitosis.conf")
	conf, err := ini.Read(confPath, ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	c.Assert(err, IsNil)
	conf.AddSection("foo bar")
	s.mngr.writeCommitPush(conf, "Some commit message")
	got := s.lastBareCommit(c)
	c.Assert(got, Equals, "Some commit message")
}

func (s *S) TestBuildAndStoreKeyFileAddsAKeyFileToTheKeydirDirectoryAndTheMemberToTheGroupAndReturnTheKeyFileName(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	keyFileName, err := m.buildAndStoreKeyFile("tolices", "my-key")
	c.Assert(err, IsNil)
	c.Assert(keyFileName, Equals, "tolices_key1.pub")
	p := s.mngr.git.getPath("keydir")
	filePath := path.Join(p, keyFileName)
	file, err := os.Open(filePath)
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
}

func (s *S) TestBuildAndStoreKeyFileUseKey2IfThereIsAlreadyAKeyForTheMember(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	p := s.mngr.git.getPath("keydir")
	key1Path := path.Join(p, "gol-de-quem_key1.pub")
	f, err := os.OpenFile(key1Path, syscall.O_CREAT, 0644)
	c.Assert(err, IsNil)
	f.Close()
	keyFileName, err := m.buildAndStoreKeyFile("gol-de-quem", "my-key")
	c.Assert(err, IsNil)
	c.Assert(keyFileName, Equals, "gol-de-quem_key2.pub")
	file, err := os.Open(path.Join(p, keyFileName))
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
}

func (s *S) TestBuildAndStoreKeyFileDoesNotReturnErrorIfTheDirectoryExists(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	p := s.mngr.git.getPath("keydir")
	os.MkdirAll(p, 0755)
	_, err = m.buildAndStoreKeyFile("vida-imbecil", "my-key")
	c.Assert(err, IsNil)
}

func (s *S) TestBuildAndStoreKeyFileCommits(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	keyfile, err := m.buildAndStoreKeyFile("the-night-and-the-silent-water", "my-key")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := fmt.Sprintf("Added %s keyfile.", keyfile)
	c.Assert(got, Equals, expected)
}

func (s *S) TesteDeleteKeyFile(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	keyfile, err := m.buildAndStoreKeyFile("blackwater-park", "my-key")
	c.Assert(err, IsNil)
	err = m.deleteKeyFile(keyfile)
	c.Assert(err, IsNil)
	p := s.mngr.git.getPath("keydir")
	keypath := path.Join(p, keyfile)
	_, err = os.Stat(keypath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TesteDeleteKeyFileReturnsErrorIfTheFileDoesNotExist(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	err = m.deleteKeyFile("dont_know.pub")
	c.Assert(err, NotNil)
}

func (s *S) TesteDeleteKeyFileCommits(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	keyfile, err := m.buildAndStoreKeyFile("windowpane", "my-key")
	c.Assert(err, IsNil)
	err = m.deleteKeyFile(keyfile)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("Deleted %s keyfile.", keyfile)
	got := s.lastBareCommit(c)
	c.Assert(got, Equals, expected)
}

func (s *S) TestCommitShouldCommitChangeInGit(c *C) {
	m, err := newGitosisManager()
	c.Assert(err, IsNil)
	f, err := os.Create(m.git.getPath("tmpfile.txt"))
	c.Assert(err, IsNil)
	f.Close()
	defer func(git *repository) {
		git.run("rm", "tmpfile.txt")
		git.commit("removed tmpfile.txt")
		git.push("origin", "master")
	}(m.git)
	err = m.commit("added tmpfile.txt")
	c.Assert(err, IsNil)
	out, err := m.git.run("log", "-1", "--format=%s")
	c.Assert(err, IsNil)
	c.Assert(out, Equals, "added tmpfile.txt\n")
}
