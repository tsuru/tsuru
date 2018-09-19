.. Copyright 2018 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Open Service Broker
+++++++++++++++++++

Overview
========

The `Open Service Broker API <https://www.openservicebrokerapi.org/>`_ project allows developers, ISVs, and SaaS vendors a single,
simple, and elegant way to deliver services to applications running within cloud native platforms.

Tsuru supports services provided by a service broker since version 1.7.0. Service brokers may be registered on the tsuru API to make
their services available for applications running on the platform. Users can create instances from these services, bind and unbind those instances
as if they were tsuru native services.

The next section explains how service brokers are managed by tsuru admins. The usage of services from those brokers do not differ from regular
tsuru services, except for the support for instance creation and binding parameters.

Managing Service Brokers
========================

To expose services from a broker, a tsuru admin needs to add the service broker endpoint to the tsuru api. This can be done on the cli.

.. highlight:: bash

::

    $ tsuru service broker add <name> <url> --token <token>

.. note::

    Refer to the command help documentation to a full list of parameters that may be set for the broker.

After adding a service broker, its services are going to be displayed just as any native tsuru service. The output below shows
the list displayed after adding the `AWS Service Broker <https://github.com/awslabs/aws-servicebroker>`_.

.. highlight:: bash

::

    $ tsuru service list
    +-----------------------+-----------+
    | Services              | Instances |
    +-----------------------+-----------+
    | aws::dh-athena        |           |
    | aws::dh-dynamodb      |           |
    | aws::dh-elasticache   |           |
    | aws::dh-emr           |           |
    | aws::dh-kinesis       |           |
    | aws::dh-kms           |           |
    | aws::dh-lex           |           |
    | aws::dh-polly         |           |
    | aws::dh-rdsmariadb    |           |
    | aws::dh-rdsmysql      |           |
    | aws::dh-rdspostgresql |           |
    | aws::dh-redshift      |           |
    | aws::dh-rekognition   |           |
    | aws::dh-route53       |           |
    | aws::dh-s3            |           |
    | aws::dh-sns           |           |
    | aws::dh-sqs           |           |
    | aws::dh-translate     |           |
    +-----------------------+-----------+


The name of each service is prefixed with the name of the broker that provides the service ("aws" in this case).
tsuru will cache the service catalog returned by the service broker for a few minutes (configurable by broker).

OSB services support creation and binding parameters. Available parameters are displayed using the ``tsuru service info`` command:

.. highlight:: bash

::

    $ tsuru service info aws::dh-route53
    Info for "aws::dh-route53"

    Plans
    +------------+-----------------------------+----------------------------------------------------------------------------------------------------------------------------------------+----------------+
    | Name       | Description                 | Instance Params                                                                                                                        | Binding Params |
    +------------+-----------------------------+----------------------------------------------------------------------------------------------------------------------------------------+----------------+
    | hostedzone | Managed Route53 hosted zone | NewHostedZoneName:                                                                                                                     |                |
    |            |                             |   description: Name of the hosted zone                                                                                                 |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | SBArtifactS3Bucket:                                                                                                                    |                |
    |            |                             |   description: Name of the S3 bucket containing the AWS Service Broker Assets                                                          |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: awsservicebroker                                                                                                            |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | SBArtifactS3KeyPrefix:                                                                                                                 |                |
    |            |                             |   description: Name of the S3 key prefix containing the AWS Service Broker Assets, leave empty if assets are in the root of the bucket |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default:                                                                                                                             |                |
    |            |                             | aws_access_key:                                                                                                                        |                |
    |            |                             |   description: AWS Access Key to authenticate to AWS with.                                                                             |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | aws_cloudformation_role_arn:                                                                                                           |                |
    |            |                             |   description: IAM role ARN for use as Cloudformation Stack Role.                                                                      |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | aws_secret_key:                                                                                                                        |                |
    |            |                             |   description: AWS Secret Key to authenticate to AWS with.                                                                             |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | region:                                                                                                                                |                |
    |            |                             |   description: AWS Region to create RDS instance in.                                                                                   |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: us-west-2                                                                                                                   |                |
    |            |                             |                                                                                                                                        |                |
    +------------+-----------------------------+----------------------------------------------------------------------------------------------------------------------------------------+----------------+
    | recordset  | Route 53 Record Set         | AliasTarget:                                                                                                                           |                |
    |            |                             |   description: Alias resource record sets only: Information about the domain to which you are redirecting traffic.                     |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | HostedZoneId:                                                                                                                          |                |
    |            |                             |   description: Id of the hosted zone which the records are to be created in                                                            |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | HostedZoneName:                                                                                                                        |                |
    |            |                             |   description: Name of the hosted zone which the records are to be created in                                                          |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | RecordName:                                                                                                                            |                |
    |            |                             |   description: Name of the record                                                                                                      |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | ResourceRecord:                                                                                                                        |                |
    |            |                             |   description: Value of the record                                                                                                     |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             | SBArtifactS3Bucket:                                                                                                                    |                |
    |            |                             |   description: Name of the S3 bucket containing the AWS Service Broker Assets                                                          |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: awsservicebroker                                                                                                            |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | SBArtifactS3KeyPrefix:                                                                                                                 |                |
    |            |                             |   description: Name of the S3 key prefix containing the AWS Service Broker Assets, leave empty if assets are in the root of the bucket |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default:                                                                                                                             |                |
    |            |                             | TimeToLive:                                                                                                                            |                |
    |            |                             |   description: How long the resolved record should be cached by resolvers                                                              |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: 360                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | Type:                                                                                                                                  |                |
    |            |                             |   description: Type of record                                                                                                          |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: A                                                                                                                           |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | aws_access_key:                                                                                                                        |                |
    |            |                             |   description: AWS Access Key to authenticate to AWS with.                                                                             |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | aws_cloudformation_role_arn:                                                                                                           |                |
    |            |                             |   description: IAM role ARN for use as Cloudformation Stack Role.                                                                      |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | aws_secret_key:                                                                                                                        |                |
    |            |                             |   description: AWS Secret Key to authenticate to AWS with.                                                                             |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   required: true                                                                                                                       |                |
    |            |                             | region:                                                                                                                                |                |
    |            |                             |   description: AWS Region to create RDS instance in.                                                                                   |                |
    |            |                             |   type: string                                                                                                                         |                |
    |            |                             |   default: us-west-2                                                                                                                   |                |
    |            |                             |                                                                                                                                        |                |
    +------------+-----------------------------+----------------------------------------------------------------------------------------------------------------------------------------+----------------+
    Documentation:
    AWS Service Broker - Amazon Route 53

An instance of this service may be created using the cli:

.. highlight:: bash

::

    $ tsuru service instance add aws::dh-route53 recordset --plan-param region=us-west-1 --plan-param aws_secret_key=XPTO

Binding, unbinding and removing the instance follows the same pattern and works just as other native services. Environment variables
returned by the service are going to also be injected into the application.

Removing a service broker may also be done by the cli:

.. highlight:: bash

::

    $ tsuru service broker delete <name>
