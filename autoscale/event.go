// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package autoscale

import (
	"time"

	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
)

const (
	scaleActionAdd       = "add"
	scaleActionRemove    = "remove"
	scaleActionRebalance = "rebalance"
)

type Event struct {
	ID            interface{} `bson:"_id"`
	MetadataValue string
	Action        string // scaleActionAdd, scaleActionRemove, scaleActionRebalance
	Reason        string // dependend on scaler
	StartTime     time.Time
	EndTime       time.Time `bson:",omitempty"`
	Successful    bool
	Error         string             `bson:",omitempty"`
	Node          provision.NodeSpec `bson:",omitempty"`
	Log           string             `bson:",omitempty"`
	Nodes         []provision.NodeSpec
}

func toAutoScaleEvent(evt *event.Event) (Event, error) {
	var data EventCustomData
	err := evt.EndData(&data)
	if err != nil {
		return Event{}, err
	}
	autoScaleEvt := Event{
		ID:            evt.UniqueID,
		MetadataValue: evt.Target.Value,
		Nodes:         data.Nodes,
		StartTime:     evt.StartTime,
		EndTime:       evt.EndTime,
		Successful:    evt.Error == "",
		Error:         evt.Error,
		Log:           evt.Log(),
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

func ListAutoScaleEvents(skip, limit int) ([]Event, error) {
	evts, err := event.List(&event.Filter{
		Skip:      skip,
		Limit:     limit,
		KindNames: []string{EventKind},
	})
	if err != nil {
		return nil, err
	}
	asEvts := make([]Event, len(evts))
	for i := range evts {
		asEvts[i], err = toAutoScaleEvent(evts[i])
		if err != nil {
			return nil, err
		}
	}
	return asEvts, nil
}
