// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
)

type autoScaleRule struct {
	MetadataFilter    string `bson:"_id"`
	Enabled           bool
	MaxContainerCount int
	ScaleDownRatio    float32
	PreventRebalance  bool
	MaxMemoryRatio    float32
}

func autoScaleRuleCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	name, err := config.GetString("docker:collection")
	if err != nil {
		return nil, err
	}
	return conn.Collection(fmt.Sprintf("%s_auto_scale_rule", name)), nil
}

func legacyAutoScaleRule() *autoScaleRule {
	metadataFilter, _ := config.GetString("docker:auto-scale:metadata-filter")
	maxContainerCount, _ := config.GetInt("docker:auto-scale:max-container-count")
	scaleDownRatio, _ := config.GetFloat("docker:auto-scale:scale-down-ratio")
	preventRebalance, _ := config.GetBool("docker:auto-scale:prevent-rebalance")
	maxUsedMemory, _ := config.GetFloat("docker:scheduler:max-used-memory")
	return &autoScaleRule{
		MaxMemoryRatio:    float32(maxUsedMemory),
		MaxContainerCount: maxContainerCount,
		MetadataFilter:    metadataFilter,
		ScaleDownRatio:    float32(scaleDownRatio),
		PreventRebalance:  preventRebalance,
		Enabled:           true,
	}
}

func autoScaleRuleForMetadata(metadataFilter string) (*autoScaleRule, error) {
	coll, err := autoScaleRuleCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	var rule autoScaleRule
	err = coll.FindId(metadataFilter).One(&rule)
	if err == mgo.ErrNotFound {
		legacyRule := legacyAutoScaleRule()
		if legacyRule.MetadataFilter == metadataFilter {
			rule = *legacyRule
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	if rule.ScaleDownRatio == 0.0 {
		rule.ScaleDownRatio = 1.333
	} else if rule.ScaleDownRatio <= 1.0 {
		return nil, fmt.Errorf("invalid rule, scale down ratio needs to be greater than 1.0, got %f", rule.ScaleDownRatio)
	}
	if rule.MaxMemoryRatio == 0.0 {
		maxMemoryRatio, _ := config.GetFloat("docker:scheduler:max-used-memory")
		rule.MaxMemoryRatio = float32(maxMemoryRatio)
	}
	return &rule, nil
}
