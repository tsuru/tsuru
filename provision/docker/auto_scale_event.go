// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"time"

	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/event"
)

const (
	scaleActionAdd       = "add"
	scaleActionRemove    = "remove"
	scaleActionRebalance = "rebalance"
)

type autoScaleEvent struct {
	ID            interface{} `bson:"_id"`
	MetadataValue string
	Action        string // scaleActionAdd, scaleActionRemove, scaleActionRebalance
	Reason        string // dependend on scaler
	StartTime     time.Time
	EndTime       time.Time `bson:",omitempty"`
	Successful    bool
	Error         string       `bson:",omitempty"`
	Node          cluster.Node `bson:",omitempty"`
	Log           string       `bson:",omitempty"`
	Nodes         []cluster.Node
}

func toAutoScaleEvent(evt *event.Event) (autoScaleEvent, error) {
	var data evtCustomData
	err := evt.EndData(&data)
	if err != nil {
		return autoScaleEvent{}, err
	}
	autoScaleEvt := autoScaleEvent{
		ID:            evt.UniqueID,
		MetadataValue: evt.Target.Value,
		Nodes:         data.Nodes,
		StartTime:     evt.StartTime,
		EndTime:       evt.EndTime,
		Successful:    evt.Error == "",
		Error:         evt.Error,
		Log:           evt.Log,
	}
	if data.Result != nil {
		if data.Result.ToAdd > 0 {
			autoScaleEvt.Action = scaleActionAdd
		} else if len(data.Result.ToRemove) > 0 {
			autoScaleEvt.Action = scaleActionRemove
		} else if data.Result.ToRebalance {
			autoScaleEvt.Action = scaleActionRebalance
		}
		autoScaleEvt.Reason = data.Result.Reason
	}
	return autoScaleEvt, nil
}

func listAutoScaleEvents(skip, limit int) ([]autoScaleEvent, error) {
	evts, err := event.List(&event.Filter{
		Skip:     skip,
		Limit:    limit,
		KindName: autoScaleEventKind,
	})
	if err != nil {
		return nil, err
	}
	asEvts := make([]autoScaleEvent, len(evts))
	for i := range evts {
		asEvts[i], err = toAutoScaleEvent(&evts[i])
		if err != nil {
			return nil, err
		}
	}
	return asEvts, nil
}
