package storage

import (
	chk "gopkg.in/check.v1"
	"math/rand"
	"strconv"
)

type StorageFileSuite struct{}

var _ = chk.Suite(&StorageFileSuite{})

func getFileClient(c *chk.C) FileServiceClient {
	return getBasicClient(c).GetFileService()
}

func (s *StorageFileSuite) Test_pathForFileShare(c *chk.C) {
	c.Assert(pathForFileShare("foo"), chk.Equals, "/foo")
}

func (s *StorageFileSuite) TestGetShareURL(c *chk.C) {
	api, err := NewBasicClient("foo", "YmFy")
	c.Assert(err, chk.IsNil)
	cli := api.GetFileService()

	c.Assert(cli.GetShareURL("share"), chk.Equals, "https://foo.file.core.windows.net/share")
}

func (s *StorageFileSuite) TestCreateShareDeleteShare(c *chk.C) {
	cli := getFileClient(c)
	name := randShare()
	c.Assert(cli.CreateShare(name), chk.IsNil)
	c.Assert(cli.DeleteShare(name), chk.IsNil)
}

func (s *StorageFileSuite) TestCreateShareIfNotExists(c *chk.C) {
	cli := getFileClient(c)
	name := randShare()
	defer cli.DeleteShare(name)

	// First create
	ok, err := cli.CreateShareIfNotExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, true)

	// Second create, should not give errors
	ok, err = cli.CreateShareIfNotExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, false)
}

func (s *StorageFileSuite) TestDeleteShareIfNotExists(c *chk.C) {
	cli := getFileClient(c)
	name := randShare()

	// delete non-existing share
	ok, err := cli.DeleteShareIfExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, false)

	c.Assert(cli.CreateShare(name), chk.IsNil)

	// delete existing share
	ok, err = cli.DeleteShareIfExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, true)
}

func (s *StorageFileSuite) Test_checkForStorageEmulator(c *chk.C) {
	f := getEmulatorClient(c).GetFileService()
	err := f.checkForStorageEmulator()
	c.Assert(err, chk.NotNil)
}

func (s *StorageFileSuite) TestListShares(c *chk.C) {
	cli := getFileClient(c)
	c.Assert(deleteTestShares(cli), chk.IsNil)

	name := randShare()

	c.Assert(cli.CreateShare(name), chk.IsNil)
	defer cli.DeleteShare(name)

	resp, err := cli.ListShares(ListSharesParameters{
		MaxResults: 5,
		Prefix:     testSharePrefix})
	c.Assert(err, chk.IsNil)

	c.Check(len(resp.Shares), chk.Equals, 1)
	c.Check(resp.Shares[0].Name, chk.Equals, name)

}

func (s *StorageFileSuite) TestShareExists(c *chk.C) {
	cli := getFileClient(c)
	name := randShare()

	ok, err := cli.ShareExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, false)

	c.Assert(cli.CreateShare(name), chk.IsNil)
	defer cli.DeleteShare(name)

	ok, err = cli.ShareExists(name)
	c.Assert(err, chk.IsNil)
	c.Assert(ok, chk.Equals, true)
}

func (s *StorageFileSuite) TestGetAndSetShareProperties(c *chk.C) {
	name := randShare()
	quota := rand.Intn(5120)

	cli := getFileClient(c)
	c.Assert(cli.CreateShare(name), chk.IsNil)
	defer cli.DeleteShare(name)

	err := cli.SetShareProperties(name, ShareHeaders{Quota: strconv.Itoa(quota)})
	c.Assert(err, chk.IsNil)

	props, err := cli.GetShareProperties(name)
	c.Assert(err, chk.IsNil)

	c.Assert(props.Quota, chk.Equals, strconv.Itoa(quota))
}

func (s *StorageFileSuite) TestGetAndSetMetadata(c *chk.C) {
	cli := getFileClient(c)
	name := randShare()

	c.Assert(cli.CreateShare(name), chk.IsNil)
	defer cli.DeleteShare(name)

	m, err := cli.GetShareMetadata(name)
	c.Assert(err, chk.IsNil)
	c.Assert(m, chk.Not(chk.Equals), nil)
	c.Assert(len(m), chk.Equals, 0)

	mPut := map[string]string{
		"foo":     "bar",
		"bar_baz": "waz qux",
	}

	err = cli.SetShareMetadata(name, mPut, nil)
	c.Assert(err, chk.IsNil)

	m, err = cli.GetShareMetadata(name)
	c.Assert(err, chk.IsNil)
	c.Check(m, chk.DeepEquals, mPut)

	// Case munging

	mPutUpper := map[string]string{
		"Foo":     "different bar",
		"bar_BAZ": "different waz qux",
	}
	mExpectLower := map[string]string{
		"foo":     "different bar",
		"bar_baz": "different waz qux",
	}

	err = cli.SetShareMetadata(name, mPutUpper, nil)
	c.Assert(err, chk.IsNil)

	m, err = cli.GetShareMetadata(name)
	c.Assert(err, chk.IsNil)
	c.Check(m, chk.DeepEquals, mExpectLower)
}

func deleteTestShares(cli FileServiceClient) error {
	for {
		resp, err := cli.ListShares(ListSharesParameters{Prefix: testSharePrefix})
		if err != nil {
			return err
		}
		if len(resp.Shares) == 0 {
			break
		}
		for _, c := range resp.Shares {
			err = cli.DeleteShare(c.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

const testSharePrefix = "zzzzztest"

func randShare() string {
	return testSharePrefix + randString(32-len(testSharePrefix))
}
