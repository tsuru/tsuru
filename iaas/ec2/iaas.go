// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/queue"
)

const defaultRegion = "us-east-1"

func init() {
	iaas.RegisterIaasProvider("ec2", newEC2IaaS)
}

type EC2IaaS struct {
	base iaas.UserDataIaaS
}

func newEC2IaaS(name string) iaas.IaaS {
	return &EC2IaaS{base: iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "ec2", IaaSName: name}}}
}

func (i *EC2IaaS) createEC2Handler(regionOrEndpoint string) (*ec2.EC2, error) {
	keyId, err := i.base.GetConfigString("key-id")
	if err != nil {
		return nil, err
	}
	secretKey, err := i.base.GetConfigString("secret-key")
	if err != nil {
		return nil, err
	}
	var region, endpoint string
	if strings.HasPrefix(regionOrEndpoint, "http") {
		endpoint = regionOrEndpoint
		region = defaultRegion
	} else {
		region = regionOrEndpoint
	}
	config := aws.Config{
		Credentials: credentials.NewStaticCredentials(keyId, secretKey, ""),
		Region:      aws.String(region),
		Endpoint:    aws.String(endpoint),
	}
	return ec2.New(session.New(&config)), nil
}

func (i *EC2IaaS) waitForDnsName(ec2Inst *ec2.EC2, instance *ec2.Instance) (*ec2.Instance, error) {
	rawWait, _ := i.base.GetConfigString("wait-timeout")
	maxWaitTime, _ := strconv.Atoi(rawWait)
	if maxWaitTime == 0 {
		maxWaitTime = 300
	}
	q, err := queue.Queue()
	if err != nil {
		return nil, err
	}
	taskName := fmt.Sprintf("ec2-wait-machine-%s", i.base.IaaSName)
	waitDuration := time.Duration(maxWaitTime) * time.Second
	job, err := q.EnqueueWait(taskName, monsterqueue.JobParams{
		"region":    ec2Inst.Config.Region,
		"endpoint":  ec2Inst.Config.Endpoint,
		"machineId": *instance.InstanceId,
		"timeout":   maxWaitTime,
	}, waitDuration)
	if err != nil {
		if err == monsterqueue.ErrQueueWaitTimeout {
			return nil, fmt.Errorf("ec2: time out after %v waiting for instance %s to start", waitDuration, *instance.InstanceId)
		}
		return nil, err
	}
	result, err := job.Result()
	if err != nil {
		return nil, err
	}
	instance.PublicDnsName = aws.String(result.(string))
	return instance, nil
}

func (i *EC2IaaS) Initialize() error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	return q.RegisterTask(&ec2WaitTask{iaas: i})
}

func (i *EC2IaaS) Describe() string {
	return `EC2 IaaS required params:
  image=<image id>         Image AMI ID
  type=<instance type>     Your template uuid

Optional params:
  region=<region>          Chosen region, defaults to us-east-1
  securityGroup=<group>    Chosen security group
  keyName=<key name>       Key name for machine
`
}

func (i *EC2IaaS) DeleteMachine(m *iaas.Machine) error {
	regionOrEndpoint := getRegionOrEndpoint(m.CreationParams, false)
	if regionOrEndpoint == "" {
		return fmt.Errorf("region or endpoint creation param required")
	}
	ec2Inst, err := i.createEC2Handler(regionOrEndpoint)
	if err != nil {
		return err
	}
	input := ec2.TerminateInstancesInput{InstanceIds: []*string{&m.Id}}
	_, err = ec2Inst.TerminateInstances(&input)
	return err
}

type invalidFieldError struct {
	fieldName    string
	convertError error
}

func (err *invalidFieldError) Error() string {
	return fmt.Sprintf("invalid value for the field %q: %s", err.fieldName, err.convertError)
}

func (i *EC2IaaS) buildRunInstancesOptions(params map[string]string) (ec2.RunInstancesInput, error) {

	result := ec2.RunInstancesInput{
		MaxCount: aws.Int64(1),
		MinCount: aws.Int64(1),
	}
	forbiddenFields := []string{
		"maxcount", "mincount", "dryrun", "blockdevicemappings",
		"iaminstanceprofile", "monitoring", "networkinterfaces",
		"placement",
	}
	aliases := map[string]string{
		"image":         "imageid",
		"type":          "instancetype",
		"securitygroup": "securitygroups",
		"ebs-optimized": "ebsoptimized",
	}
	refType := reflect.TypeOf(result)
	refValue := reflect.ValueOf(&result)
	for key, value := range params {
		field, ok := refType.FieldByNameFunc(func(name string) bool {
			lowerName := strings.ToLower(name)
			for _, field := range forbiddenFields {
				if lowerName == field {
					return false
				}
			}
			lowerKey := strings.ToLower(key)
			if aliased, ok := aliases[lowerKey]; ok {
				lowerKey = aliased
			}
			return lowerName == lowerKey
		})
		if !ok {
			continue
		}
		fieldType := field.Type
		fieldValue := refValue.Elem().FieldByIndex(field.Index)
		if !fieldValue.IsValid() || !fieldValue.CanSet() {
			continue
		}
		switch fieldType.Kind() {
		case reflect.Ptr:
			switch fieldType.Elem().Kind() {
			case reflect.String:
				copy := value
				fieldValue.Set(reflect.ValueOf(&copy))
			case reflect.Int64:
				intValue, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return result, &invalidFieldError{
						fieldName:    key,
						convertError: err,
					}
				}
				fieldValue.Set(reflect.ValueOf(&intValue))
			case reflect.Bool:
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return result, &invalidFieldError{
						fieldName:    key,
						convertError: err,
					}
				}
				fieldValue.Set(reflect.ValueOf(&boolValue))
			}
		case reflect.Slice:
			parts := strings.Split(value, ",")
			values := make([]*string, len(parts))
			for i, part := range parts {
				values[i] = aws.String(part)
			}
			fieldValue.Set(reflect.ValueOf(values))
		}
	}
	return result, nil
}

func (i *EC2IaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	regionOrEndpoint := getRegionOrEndpoint(params, true)
	userData, err := i.base.ReadUserData()
	if err != nil {
		return nil, err
	}
	options, err := i.buildRunInstancesOptions(params)
	if err != nil {
		return nil, err
	}
	options.UserData = aws.String(base64.StdEncoding.EncodeToString([]byte(userData)))
	if options.ImageId == nil || *options.ImageId == "" {
		return nil, fmt.Errorf("the parameter %q is required", "imageid")
	}
	if options.InstanceType == nil || *options.InstanceType == "" {
		return nil, fmt.Errorf("the parameter %q is required", "instancetype")
	}
	ec2Inst, err := i.createEC2Handler(regionOrEndpoint)
	if err != nil {
		return nil, err
	}
	resp, err := ec2Inst.RunInstances(&options)
	if err != nil {
		return nil, err
	}
	if len(resp.Instances) == 0 {
		return nil, fmt.Errorf("no instance created")
	}
	runInst := resp.Instances[0]
	if tags, ok := params["tags"]; ok {
		var ec2Tags []*ec2.Tag
		tagList := strings.Split(tags, ",")
		ec2Tags = make([]*ec2.Tag, 0, len(tagList))
		for _, tag := range tagList {
			if strings.Contains(tag, ":") {
				parts := strings.SplitN(tag, ":", 2)
				ec2Tags = append(ec2Tags, &ec2.Tag{
					Key:   aws.String(parts[0]),
					Value: aws.String(parts[1]),
				})
			}
		}
		if len(ec2Tags) > 0 {
			input := ec2.CreateTagsInput{
				Resources: []*string{runInst.InstanceId},
				Tags:      ec2Tags,
			}
			_, err = ec2Inst.CreateTags(&input)
			if err != nil {
				log.Errorf("failed to tag EC2 instance: %s", err)
			}
		}
	}
	instance, err := i.waitForDnsName(ec2Inst, runInst)
	if err != nil {
		return nil, err
	}
	machine := iaas.Machine{
		Id:      *instance.InstanceId,
		Status:  *instance.State.Name,
		Address: *instance.PublicDnsName,
	}
	return &machine, nil
}

func getRegionOrEndpoint(params map[string]string, useDefault bool) string {
	regionOrEndpoint := params["endpoint"]
	if regionOrEndpoint == "" {
		regionOrEndpoint = params["region"]
		if regionOrEndpoint == "" && useDefault {
			regionOrEndpoint = defaultRegion
		}
	}
	return regionOrEndpoint
}
