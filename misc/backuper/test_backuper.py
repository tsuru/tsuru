import unittest

from mock import patch, call

import backuper


class TestBackuper(unittest.TestCase):

    @patch("boto.ec2.connection.EC2Connection")
    def test_backup_should_create_image_in_ec2(self, mock):
        instance = mock.return_value
        instance.create_image.return_value = "test-ami-1"
        images = backuper.backup("access", "secret", ["i-1"])
        mock.assert_called_with("access", "secret")
        self.assertListEqual(["test-ami-1"], images)

    @patch("boto.ec2.connection.EC2Connection")
    def test_snapshot_calls_ec2_create_image_with_right_instances(self, mock):
        instance = mock.return_value
        instance.create_image.return_value = "test-ami-2"
        image_ids = backuper.snapshot(instance, ["i-2"])
        instance.create_image.assert_called_once_with("i-2", "i-2-snapshot")
        self.assertListEqual(["test-ami-2"], image_ids)

    @patch("boto.ec2.connection.EC2Connection")
    def test_snapshot_snapshots_multiple_instances(self, mock):
        instance = mock.return_value
        instance.create_image.return_value = "test-ami-1"
        image_ids = backuper.snapshot(instance, ["i-2", "i-3"])
        self.assertEqual(instance.create_image.call_count, 2)
        calls = [call("i-2", "i-2-snapshot"), call("i-3", "i-3-snapshot")]
        instance.create_image.assert_has_calls(calls, any_order=False)
        self.assertListEqual(["test-ami-1", "test-ami-1"], image_ids)


if __name__ == "__main__":
    unittest.main()
