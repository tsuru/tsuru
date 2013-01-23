def backup(access, secret, instances):
    """takes a snapshot of a list of instances and stores it on database"""
    from boto.ec2.connection import EC2Connection
    conn = EC2Connection(access, secret)
    image_ids = snapshot(conn, instances)
    remove_old(instances)
    save_current(image_ids)
    return image_ids


def snapshot(conn, instance_ids):
    """takes a snapshot of the given VMs"""
    image_ids = []
    for instance_id in instance_ids:
        image_id = conn.create_image(instance_id, "{0}-snapshot".format(instance_id))
        image_ids.append(image_id)
    return image_ids


def remove_old(instances):
    """removes old images from the database to give room for the new ones"""
    pass


def save_current(image_ids):
    """stores the given image ids in the databse"""
    pass


def clean():
    """compares the images on s3 with the images on database,
    if there are extra images in the database, delete them"""
    pass


if __name__ == "__main__":
    backup(access="", secret="", instances=["i-2", "i-3"])
