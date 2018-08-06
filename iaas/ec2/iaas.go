// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ec2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
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
		HTTPClient:  tsuruNet.Dial15Full300ClientNoKeepAlive,
	}
	newSession, err := session.NewSession(&config)
	if err != nil {
		return nil, err
	}
	return ec2.New(newSession), nil
}

func (i *EC2IaaS) waitForDnsName(ec2Inst *ec2.EC2, instanceID string, createParams map[string]string) (string, error) {
	rawWait, _ := i.base.GetConfigString("wait-timeout")
	maxWaitTime, _ := strconv.Atoi(rawWait)
	if maxWaitTime == 0 {
		maxWaitTime = 300
	}
	q, err := queue.Queue()
	if err != nil {
		return "", err
	}
	taskName := fmt.Sprintf("ec2-wait-machine-%s", i.base.IaaSName)
	jobParams := monsterqueue.JobParams{
		"region":    ec2Inst.Config.Region,
		"endpoint":  ec2Inst.Config.Endpoint,
		"machineId": instanceID,
		"timeout":   maxWaitTime,
	}
	if rawInterfaceIdx, ok := createParams["network-index"]; ok {
		if interfaceIdx, atoiErr := strconv.Atoi(rawInterfaceIdx); atoiErr == nil {
			jobParams["networkIndex"] = interfaceIdx
		}
	}
	waitDuration := time.Duration(maxWaitTime) * time.Second
	job, err := q.EnqueueWait(taskName, jobParams, waitDuration)
	if err != nil {
		if err == monsterqueue.ErrQueueWaitTimeout {
			return "", errors.Errorf("ec2: time out after %v waiting for instance %s to start", waitDuration, instanceID)
		}
		return "", err
	}
	result, err := job.Result()
	if err != nil {
		return "", err
	}
	return result.(string), nil
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
		return errors.Errorf("region or endpoint creation param required")
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
		"maxcount", "mincount", "dryrun", "monitoring",
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
			case reflect.Struct:
				err := i.loadStruct(fieldValue, fieldType, []byte(value))
				if err != nil {
					return result, &invalidFieldError{
						fieldName:    key,
						convertError: err,
					}
				}
			}
		case reflect.Slice:
			switch fieldType.Elem().Elem().Kind() {
			case reflect.String:
				parts := strings.Split(value, ",")
				values := make([]*string, len(parts))
				for i, part := range parts {
					values[i] = aws.String(part)
				}
				fieldValue.Set(reflect.ValueOf(values))
			case reflect.Struct:
				var raw []map[string]interface{}
				err := json.Unmarshal([]byte(value), &raw)
				if err != nil {
					return result, &invalidFieldError{
						fieldName:    key,
						convertError: err,
					}
				}
				val, err := i.translateSlice(raw, fieldType)
				if err != nil {
					return result, &invalidFieldError{
						fieldName:    key,
						convertError: err,
					}
				}
				fieldValue.Set(val)
			}
		}
	}

	// Manual configuration
	if monitoring, ok := params["monitoring-enabled"]; ok {
		value, _ := strconv.ParseBool(monitoring)
		result.Monitoring = &ec2.RunInstancesMonitoringEnabled{
			Enabled: aws.Bool(value),
		}
	}

	return result, nil
}

func (i *EC2IaaS) translateSlice(in []map[string]interface{}, t reflect.Type) (reflect.Value, error) {
	result := reflect.MakeSlice(t, len(in), len(in))
	for idx, value := range in {
		data, _ := json.Marshal(value)
		err := i.loadStruct(result.Index(idx), t.Elem(), data)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (i *EC2IaaS) loadStruct(value reflect.Value, t reflect.Type, data []byte) error {
	raw := value.Interface()
	if value.IsNil() {
		raw = reflect.New(t.Elem()).Interface()
	}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	value.Set(reflect.ValueOf(raw))
	return nil
}

func (i *EC2IaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	regionOrEndpoint := getRegionOrEndpoint(params, true)
	userData, err := i.base.ReadUserData(params)
	if err != nil {
		return nil, err
	}
	options, err := i.buildRunInstancesOptions(params)
	if err != nil {
		return nil, err
	}
	options.UserData = aws.String(base64.StdEncoding.EncodeToString([]byte(userData)))
	if options.ImageId == nil || *options.ImageId == "" {
		return nil, errors.Errorf("the parameter %q is required", "imageid")
	}
	if options.InstanceType == nil || *options.InstanceType == "" {
		return nil, errors.Errorf("the parameter %q is required", "instancetype")
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
		return nil, errors.Errorf("no instance created")
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
	dnsName, err := i.waitForDnsName(ec2Inst, aws.StringValue(runInst.InstanceId), params)
	if err != nil {
		return nil, err
	}
	machine := iaas.Machine{
		Id:      aws.StringValue(runInst.InstanceId),
		Status:  aws.StringValue(runInst.State.Name),
		Address: dnsName,
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
