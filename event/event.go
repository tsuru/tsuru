// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	eventTypes "github.com/tsuru/tsuru/types/event"

	permTypes "github.com/tsuru/tsuru/types/permission"
	"go.mongodb.org/mongo-driver/bson"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	eventDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_events_duration_seconds",
		Help:    "The duration of events in seconds",
		Buckets: []float64{1, 5, 10, 60, 600, 1800},
	}, []string{"kind"})

	eventCurrent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tsuru_events_current",
		Help: "The number of events currently running",
	}, []string{"kind"})

	eventsRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_events_rejected_total",
		Help: "The total number of events rejected",
	}, []string{"kind", "reason"})

	eventsExpired = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_events_expired_total",
		Help: "The total number of events expired",
	}, []string{"kind"})

	defaultAppRetryTimeout = 10 * time.Second
)

const (
	rejectLocked    = "locked"
	rejectBlocked   = "blocked"
	rejectThrottled = "throttled"

	timeFormat = "2006-01-02 15:04:05 -0700"
)

var (
	throttlingInfo  = map[string]ThrottlingSpec{}
	errInvalidQuery = errors.New("invalid query")

	ErrNotCancelable          = errors.New("event is not cancelable")
	ErrCancelAlreadyRequested = errors.New("event cancel already requested")
	ErrEventNotFound          = errors.New("event not found")
	ErrNoTarget               = ErrValidation("event target is mandatory")
	ErrNoKind                 = ErrValidation("event kind is mandatory")
	ErrNoOwner                = ErrValidation("event owner is mandatory")
	ErrNoOpts                 = ErrValidation("event opts is mandatory")
	ErrNoInternalKind         = ErrValidation("event internal kind is mandatory")
	ErrNoAllowed              = errors.New("event allowed is mandatory")
	ErrNoAllowedCancel        = errors.New("event allowed cancel is mandatory for cancelable events")
	ErrInvalidOwner           = ErrValidation("event owner must not be set on internal events")
	ErrInvalidKind            = ErrValidation("event kind must not be set on internal events")
)

const (
	filterMaxLimit = 100
)

func init() {
	prometheus.MustRegister(eventDuration, eventCurrent, eventsRejected)
}

type ErrThrottled struct {
	Spec       *ThrottlingSpec
	Target     eventTypes.Target
	AllTargets bool
}

func (err ErrThrottled) Error() string {
	var extra string
	if err.Spec.KindName != "" {
		extra = fmt.Sprintf(" %s on", err.Spec.KindName)
	}
	var extraTarget string
	if err.AllTargets {
		extraTarget = fmt.Sprintf("any %s", err.Target.Type)
	} else {
		extraTarget = fmt.Sprintf("%s %q", err.Target.Type, err.Target.Value)
	}
	return fmt.Sprintf("event throttled, limit for%s %s is %d every %v", extra, extraTarget, err.Spec.Max, err.Spec.Time)
}

type ErrValidation string

func (err ErrValidation) Error() string {
	return string(err)
}

type ErrEventLocked struct{ Event *Event }

func (err ErrEventLocked) Error() string {
	return fmt.Sprintf("event locked: %v", err.Event)
}

type ThrottlingSpec struct {
	TargetType eventTypes.TargetType `json:"target-type"`

	KindName   string        `json:"kind-name"`
	Max        int           `json:"limit"`
	Time       time.Duration `json:"window"`
	AllTargets bool          `json:"all-targets"`
	WaitFinish bool          `json:"wait-finish"`
}

func (d *ThrottlingSpec) UnmarshalJSON(data []byte) error {
	type throttlingSpecAlias ThrottlingSpec
	var v throttlingSpecAlias
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	*d = ThrottlingSpec(v)
	d.Time = d.Time * time.Second
	return nil
}

func throttlingKey(targetType eventTypes.TargetType, kindName string, allTargets bool) string {
	key := string(targetType)
	if kindName != "" {
		key = fmt.Sprintf("%s_%s", key, kindName)
	}
	if allTargets {
		key = fmt.Sprintf("%s_%s", key, "global")
	}
	return key
}

func Initialize() error {
	err := loadThrottling()
	if err != nil {
		return errors.Wrap(err, "unable to load event throttling")
	}
	cleaner.start()
	return nil
}

func loadThrottling() error {
	var specs []ThrottlingSpec
	err := internalConfig.UnmarshalConfig("event:throttling", &specs)
	if err != nil {
		if _, isNotFound := errors.Cause(err).(config.ErrKeyNotFound); isNotFound {
			return nil
		}
		return err
	}
	for _, spec := range specs {
		SetThrottling(spec)
	}
	return nil
}

func SetThrottling(spec ThrottlingSpec) {
	key := throttlingKey(spec.TargetType, spec.KindName, spec.AllTargets)
	throttlingInfo[key] = spec
}

func getThrottling(t *eventTypes.Target, k *eventTypes.Kind, allTargets bool) *ThrottlingSpec {
	keys := []string{
		throttlingKey(t.Type, k.Name, allTargets),
		throttlingKey(t.Type, "", allTargets),
	}
	for _, key := range keys {
		if s, ok := throttlingInfo[key]; ok {
			return &s
		}
	}
	return nil
}

type Event struct {
	eventTypes.EventData
	logMu     sync.Mutex
	logWriter io.Writer
}

type Opts struct {
	Target        eventTypes.Target
	ExtraTargets  []eventTypes.ExtraTarget
	Kind          *permTypes.PermissionScheme
	InternalKind  string
	Owner         auth.Token
	RawOwner      eventTypes.Owner
	RemoteAddr    string
	CustomData    interface{}
	DisableLock   bool
	Cancelable    bool
	Allowed       eventTypes.AllowedPermission
	AllowedCancel eventTypes.AllowedPermission
	RetryTimeout  time.Duration
	ExpireAt      *time.Time
}

func Allowed(scheme *permTypes.PermissionScheme, contexts ...permTypes.PermissionContext) eventTypes.AllowedPermission {
	return eventTypes.AllowedPermission{
		Scheme:   scheme.FullName(),
		Contexts: contexts,
	}
}

func (e *Event) String() string {
	return fmt.Sprintf("%s(%s) running %q start by %s at %s",
		e.Target.Type,
		e.Target.Value,
		e.Kind,
		e.Owner,
		e.StartTime.Format(time.RFC3339),
	)
}

type TargetFilter struct {
	Type   eventTypes.TargetType
	Values []string
}

type Filter struct {
	Target         eventTypes.Target
	KindType       eventTypes.KindType
	KindNames      []string `form:"-"`
	OwnerType      eventTypes.OwnerType
	OwnerName      string
	Since          time.Time
	Until          time.Time
	Running        *bool
	ErrorOnly      bool
	Raw            mongoBSON.M
	AllowedTargets []TargetFilter
	Permissions    []permTypes.Permission

	Limit int
	Skip  int
	Sort  string
}

func (f *Filter) PruneUserValues() {
	f.Raw = nil
	f.AllowedTargets = nil
	f.Permissions = nil
	if f.Limit > filterMaxLimit || f.Limit <= 0 {
		f.Limit = filterMaxLimit
	}
}

func (f *Filter) LoadKindNames(form map[string][]string) {
	for k, values := range form {
		if strings.ToLower(k) != "kindname" {
			continue
		}
		for _, val := range values {
			if val != "" {
				f.KindNames = append(f.KindNames, val)
			}
		}
	}
}

func (f *Filter) toQuery() (mongoBSON.M, error) {
	query := mongoBSON.M{}
	permMap := map[string][]permTypes.PermissionContext{}
	andBlock := []mongoBSON.M{}
	if f.Permissions != nil {
		for _, p := range f.Permissions {
			permMap[p.Scheme.FullName()] = append(permMap[p.Scheme.FullName()], p.Context)
		}
		var permOrBlock []mongoBSON.M
		for perm, ctxs := range permMap {
			ctxsBson := []mongoBSON.D{}
			for _, ctx := range ctxs {
				if ctx.CtxType == permTypes.CtxGlobal {
					ctxsBson = nil
					break
				}
				ctxsBson = append(ctxsBson, mongoBSON.D{
					{Key: "ctxtype", Value: ctx.CtxType},
					{Key: "value", Value: ctx.Value},
				})
			}
			toAppend := mongoBSON.M{
				"allowed.scheme": mongoBSON.M{"$regex": "^" + strings.Replace(perm, ".", `\.`, -1)},
			}
			if ctxsBson != nil {
				toAppend["allowed.contexts"] = mongoBSON.M{"$in": ctxsBson}
			}
			permOrBlock = append(permOrBlock, toAppend)
		}
		andBlock = append(andBlock, mongoBSON.M{"$or": permOrBlock})
	}
	if f.AllowedTargets != nil {
		var orBlock []mongoBSON.M
		for _, at := range f.AllowedTargets {
			f := mongoBSON.M{"target.type": at.Type}
			extraF := mongoBSON.M{"extratargets.target.type": at.Type}
			if at.Values != nil {
				f["target.value"] = mongoBSON.M{"$in": at.Values}
				extraF["extratargets.target.value"] = mongoBSON.M{"$in": at.Values}
			}
			orBlock = append(orBlock, f, extraF)
		}
		if len(orBlock) == 0 {
			return nil, errInvalidQuery
		}
		andBlock = append(andBlock, mongoBSON.M{"$or": orBlock})
	}
	if f.Target.Type != "" {
		orBlock := []mongoBSON.M{
			{"target.type": f.Target.Type},
			{"extratargets.target.type": f.Target.Type},
		}
		andBlock = append(andBlock, mongoBSON.M{"$or": orBlock})
	}
	if f.Target.Value != "" {
		orBlock := []mongoBSON.M{
			{"target.value": f.Target.Value},
			{"extratargets.target.value": f.Target.Value},
		}
		andBlock = append(andBlock, mongoBSON.M{"$or": orBlock})
	}
	if f.KindType != "" {
		query["kind.type"] = f.KindType
	}
	if len(f.KindNames) > 0 {
		query["kind.name"] = mongoBSON.M{"$in": f.KindNames}
	}
	if f.OwnerType != "" {
		query["owner.type"] = f.OwnerType
	}
	if f.OwnerName != "" {
		query["owner.name"] = f.OwnerName
	}
	var timeParts []mongoBSON.M
	if !f.Since.IsZero() {
		timeParts = append(timeParts, mongoBSON.M{"starttime": mongoBSON.M{"$gte": f.Since}})
	}
	if !f.Until.IsZero() {
		timeParts = append(timeParts, mongoBSON.M{"starttime": mongoBSON.M{"$lte": f.Until}})
	}
	if len(timeParts) != 0 {
		andBlock = append(andBlock, timeParts...)
	}
	if len(andBlock) > 0 {
		query["$and"] = andBlock
	}
	if f.Running != nil {
		query["running"] = *f.Running
	}
	if f.ErrorOnly {
		query["error"] = mongoBSON.M{"$ne": ""}
	}
	if f.Raw != nil {
		for k, v := range f.Raw {
			query[k] = v
		}
	}
	return query, nil
}

func GetKinds(ctx context.Context) ([]eventTypes.Kind, error) {
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return nil, err
	}
	var kinds []eventTypes.Kind

	values, err := collection.Distinct(ctx, "kind", mongoBSON.M{}, options.Distinct())
	if err != nil {
		return nil, err
	}

	for _, value := range values {
		rawD, ok := value.(primitive.D)

		if !ok {
			continue
		}

		var kind eventTypes.Kind

		for _, elem := range rawD {
			if elem.Key == "type" {
				kind.Type = eventTypes.KindType(elem.Value.(string))
			} else if elem.Key == "name" {
				kind.Name = elem.Value.(string)
			}
		}

		kinds = append(kinds, kind)
	}

	return kinds, nil
}

func transformEvent(data eventTypes.EventData) *Event {
	var event Event
	event.EventData = data
	event.fillLegacyLog()
	return &event
}

func GetRunning(ctx context.Context, target eventTypes.Target, kind string) (*Event, error) {
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return nil, err
	}
	var evtData eventTypes.EventData
	err = collection.FindOne(ctx, mongoBSON.M{
		"lock":      target,
		"kind.name": kind,
		"running":   true,
	}).Decode(&evtData)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	evt := transformEvent(evtData)
	return evt, nil
}

func GetByHexID(ctx context.Context, hexid string) (*Event, error) {
	objectID, err := primitive.ObjectIDFromHex(hexid)
	if err != nil {
		return nil, errors.Errorf("received ID is not a valid event object id: %q", hexid)
	}

	return GetByID(ctx, objectID)
}

func GetByID(ctx context.Context, id primitive.ObjectID) (*Event, error) {
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return nil, err
	}
	var evtData eventTypes.EventData
	err = collection.FindOne(ctx, mongoBSON.M{
		"uniqueid": id,
	}).Decode(&evtData)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	evt := transformEvent(evtData)
	return evt, nil
}

func EventInfo(event *Event) (*eventTypes.EventInfo, error) {
	startCustomData, err := bsonToNative(event.StartCustomData)
	if err != nil {
		return nil, err
	}

	endCustomData, err := bsonToNative(event.EndCustomData)
	if err != nil {
		return nil, err
	}

	otherCustomData, err := bsonToNative(event.OtherCustomData)
	if err != nil {
		return nil, err
	}

	return &eventTypes.EventInfo{
		EventData: event.EventData,
		StartCustomData: eventTypes.LegacyBSONRaw{
			Kind: byte(event.StartCustomData.Type),
			Data: event.EventData.StartCustomData.Value,
		},
		EndCustomData: eventTypes.LegacyBSONRaw{
			Kind: byte(event.EndCustomData.Type),
			Data: event.EventData.EndCustomData.Value,
		},
		OtherCustomData: eventTypes.LegacyBSONRaw{
			Kind: byte(event.OtherCustomData.Type),
			Data: event.EventData.OtherCustomData.Value,
		},
		CustomData: eventTypes.EventInfoCustomData{
			Start: startCustomData,
			End:   endCustomData,
			Other: otherCustomData,
		},
	}, nil
}

func bsonToNative(raw mongoBSON.RawValue) (any, error) {
	if raw.Type == 0 {
		return nil, nil
	}
	if raw.Type == bson.TypeEmbeddedDocument {
		data := map[string]any{}

		err := raw.Unmarshal(&data)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	if raw.Type == bson.TypeArray {
		data := []map[string]any{}

		err := raw.Unmarshal(&data)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	var genericData any
	err := raw.Unmarshal(&genericData)
	if err != nil {
		return nil, err
	}

	return genericData, nil
}

func All(ctx context.Context) ([]*Event, error) {
	return List(ctx, nil)
}

func List(ctx context.Context, filter *Filter) ([]*Event, error) {
	limit := 0
	skip := 0
	var query mongoBSON.M
	var err error
	sort := mongoBSON.M{"starttime": -1}
	if filter != nil {
		limit = filterMaxLimit
		if filter.Limit != 0 {
			limit = filter.Limit
		}
		if strings.HasPrefix(filter.Sort, "-") {
			sort = mongoBSON.M{filter.Sort[1:]: -1}
		} else if filter.Sort != "" {
			sort = mongoBSON.M{filter.Sort: 1}
		}
		if filter.Skip > 0 {
			skip = filter.Skip
		}
		query, err = filter.toQuery()
		if err != nil {
			if err == errInvalidQuery {
				return nil, nil
			}
			return nil, err
		}
	}
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return nil, err
	}

	options := options.Find().SetSort(sort)
	if limit > 0 {
		options = options.SetLimit(int64(limit))
	}
	if skip > 0 {
		options = options.SetSkip(int64(skip))
	}

	cursor, err := collection.Find(ctx, query, options)
	if err != nil {
		return nil, err
	}

	var allData []eventTypes.EventData
	err = cursor.All(ctx, &allData)
	if err != nil {
		return nil, err
	}
	evts := make([]*Event, len(allData))
	for i := range evts {
		evts[i] = transformEvent(allData[i])
	}
	return evts, nil
}

func New(ctx context.Context, opts *Opts) (*Event, error) {
	if opts == nil {
		return nil, ErrNoOpts
	}
	if opts.Owner == nil && opts.RawOwner.Name == "" && opts.RawOwner.Type == "" {
		return nil, ErrNoOwner
	}
	if opts.Kind == nil {
		return nil, ErrNoKind
	}
	if opts.RetryTimeout == 0 && opts.Target.Type == eventTypes.TargetTypeApp {
		opts.RetryTimeout = defaultAppRetryTimeout
	}
	return newEvt(ctx, opts)
}

func NewInternal(ctx context.Context, opts *Opts) (*Event, error) {
	if opts == nil {
		return nil, ErrNoOpts
	}
	if opts.Owner != nil {
		return nil, ErrInvalidOwner
	}
	if opts.Kind != nil {
		return nil, ErrInvalidKind
	}
	if opts.InternalKind == "" {
		return nil, ErrNoInternalKind
	}
	return newEvt(ctx, opts)
}

func makeBSONRaw(in interface{}) (mongoBSON.RawValue, error) {
	if in == nil {
		return mongoBSON.RawValue{}, nil
	}
	kind, data, err := mongoBSON.MarshalValue(in)
	if err != nil {
		return mongoBSON.RawValue{}, err
	}
	return mongoBSON.RawValue{Type: kind, Value: data}, nil
}

func checkThrottling(ctx context.Context, collection *mongo.Collection, target *eventTypes.Target, kind *eventTypes.Kind, allTargets bool) error {
	tSpec := getThrottling(target, kind, allTargets)
	if tSpec == nil || tSpec.Max <= 0 || tSpec.Time <= 0 {
		return nil
	}
	query := mongoBSON.M{
		"target.type": target.Type,
	}
	now := time.Now().UTC()
	startTimeQuery := mongoBSON.M{"$gt": now.Add(-tSpec.Time)}
	if tSpec.WaitFinish {
		query["$or"] = []mongoBSON.M{
			{"starttime": startTimeQuery},
			{
				"running":        true,
				"lockupdatetime": mongoBSON.M{"$gt": now.Add(-lockExpireTimeout)},
			},
		}
	} else {
		query["starttime"] = startTimeQuery
	}
	if !allTargets {
		query["target.value"] = target.Value
	}
	if tSpec.KindName != "" {
		query["kind.name"] = tSpec.KindName
	}

	c, err := collection.CountDocuments(ctx, query)
	if err != nil {
		return err
	}
	if c >= int64(tSpec.Max) {
		return ErrThrottled{Spec: tSpec, Target: *target, AllTargets: allTargets}
	}
	return nil
}

func newEvt(ctx context.Context, opts *Opts) (evt *Event, err error) {
	if opts.RetryTimeout == 0 {
		return newEvtOnce(ctx, opts)
	}

	timeoutCh := time.After(opts.RetryTimeout)
	for {
		evt, err := newEvtOnce(ctx, opts)
		if err == nil {
			return evt, nil
		}
		if _, ok := err.(ErrEventLocked); !ok {
			return nil, err
		}
		select {
		case <-timeoutCh:
			return nil, err
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func newEvtOnce(ctx context.Context, opts *Opts) (evt *Event, err error) {
	var k eventTypes.Kind
	defer func() {
		eventCurrent.WithLabelValues(k.Name).Inc()
		if err != nil {
			reason := "other"
			switch err.(type) {
			case ErrEventLocked:
				reason = rejectLocked
			case ErrEventBlocked:
				reason = rejectBlocked
			case ErrThrottled:
				reason = rejectThrottled
			}
			if !(reason == rejectBlocked) {
				eventCurrent.WithLabelValues(k.Name).Dec()
			}
			eventsRejected.WithLabelValues(k.Name, reason).Inc()
			return
		}
	}()

	updater.start()

	if opts == nil {
		return nil, ErrNoOpts
	}
	if !opts.Target.IsValid() {
		return nil, ErrNoTarget
	}
	if opts.Allowed.Scheme == "" && len(opts.Allowed.Contexts) == 0 {
		return nil, ErrNoAllowed
	}
	if opts.Cancelable && opts.AllowedCancel.Scheme == "" && len(opts.AllowedCancel.Contexts) == 0 {
		return nil, ErrNoAllowedCancel
	}
	if opts.Kind == nil {
		if opts.InternalKind == "" {
			return nil, ErrNoKind
		}
		k.Type = eventTypes.KindTypeInternal
		k.Name = opts.InternalKind
	} else {
		k.Type = eventTypes.KindTypePermission
		k.Name = opts.Kind.FullName()
	}
	var o eventTypes.Owner
	if opts.Owner == nil {
		if opts.RawOwner.Name != "" && opts.RawOwner.Type != "" {
			o = opts.RawOwner
		} else {
			o.Type = eventTypes.OwnerTypeInternal
		}
	} else {
		if token, ok := opts.Owner.(authTypes.NamedToken); ok {
			o.Type = eventTypes.OwnerTypeToken
			o.Name = token.GetTokenName()
		} else {
			o.Type = eventTypes.OwnerTypeUser
			o.Name = opts.Owner.GetUserName()
		}
	}

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return nil, err
	}
	err = checkThrottling(ctx, collection, &opts.Target, &k, false)
	if err != nil {
		return nil, err
	}
	err = checkThrottling(ctx, collection, &opts.Target, &k, true)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	raw, err := makeBSONRaw(opts.CustomData)
	if err != nil {
		return nil, err
	}
	uniqID := primitive.NewObjectID()

	instance, err := servicemanager.InstanceTracker.CurrentInstance(context.TODO())
	if err != nil {
		return nil, err
	}

	sourceIP := ""
	if opts.RemoteAddr != "" {
		sourceIP, _, _ = net.SplitHostPort(opts.RemoteAddr)
	}

	evt = &Event{
		EventData: eventTypes.EventData{
			ID:              uniqID,
			UniqueID:        uniqID,
			ExtraTargets:    opts.ExtraTargets,
			Target:          opts.Target,
			StartTime:       now,
			Kind:            k,
			Owner:           o,
			SourceIP:        sourceIP,
			StartCustomData: raw,
			LockUpdateTime:  now,
			Running:         true,
			Cancelable:      opts.Cancelable,
			Allowed:         opts.Allowed,
			AllowedCancel:   opts.AllowedCancel,
			Instance:        instance,
		},
	}

	if !opts.DisableLock {
		evt.EventData.Lock = &opts.Target
	}
	if opts.ExpireAt != nil {
		evt.EventData.ExpireAt = *opts.ExpireAt
	}

	maxRetries := 1
	for i := 0; i < maxRetries+1; i++ {
		_, err = collection.InsertOne(ctx, evt.EventData)

		if err == nil {
			err = checkLocked(ctx, evt, opts.DisableLock)
			if err != nil {
				evt.Abort(context.TODO())
				return nil, err
			}
			err = checkIsBlocked(ctx, evt)
			if err != nil {
				evt.Done(context.TODO(), err)
				return nil, err
			}
			updater.add(uniqID)
			return evt, nil
		}

		if mongo.IsDuplicateKeyError(err) {
			if i >= maxRetries || !checkIsExpired(ctx, collection, evt.Lock) {
				var existing Event

				err = collection.FindOne(ctx, mongoBSON.M{"lock": evt.Lock}).Decode(&existing.EventData)
				if err == mongo.ErrNoDocuments {
					maxRetries++
				}
				if err == nil {
					err = ErrEventLocked{Event: &existing}
				}
			}
		} else {
			return nil, err
		}
	}
	return nil, err
}

func checkLocked(ctx context.Context, evt *Event, disableLock bool) error {
	var targets []eventTypes.Target
	if !disableLock {
		targets = append(targets, evt.Target)
	}
	for _, et := range evt.ExtraTargets {
		if et.Lock {
			targets = append(targets, et.Target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	var orBlock []mongoBSON.M
	for _, t := range targets {
		tBson := mongoBSON.D{
			{Key: "type", Value: t.Type},
			{Key: "value", Value: t.Value},
		}
		orBlock = append(orBlock, mongoBSON.M{"lock": tBson}, mongoBSON.M{
			"extratargets": mongoBSON.M{"$elemMatch": mongoBSON.M{"target": tBson, "lock": true}},
		})
	}

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}
	var existing Event
	err = collection.FindOne(ctx, mongoBSON.M{
		"running": true,
		"_id":     mongoBSON.M{"$ne": evt.ID},
		"$or":     orBlock,
	}).Decode(&existing.EventData)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return err
	}
	return ErrEventLocked{Event: &existing}
}

func (e *Event) RawInsert(ctx context.Context, start, other, end interface{}) error {
	var err error
	e.StartCustomData, err = makeBSONRaw(start)
	if err != nil {
		return err
	}
	e.OtherCustomData, err = makeBSONRaw(other)
	if err != nil {
		return err
	}
	e.EndCustomData, err = makeBSONRaw(end)
	if err != nil {
		return err
	}
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}

	e.logMu.Lock()
	defer e.logMu.Unlock()
	_, err = collection.InsertOne(ctx, e.EventData)
	return err
}

func (e *Event) Abort(ctx context.Context) error {
	return e.done(ctx, nil, nil, true)
}

func (e *Event) Done(ctx context.Context, evtErr error) error {
	return e.done(ctx, evtErr, nil, false)
}

func (e *Event) DoneCustomData(ctx context.Context, evtErr error, customData interface{}) error {
	return e.done(ctx, evtErr, customData, false)
}

func (e *Event) SetLogWriter(w io.Writer) {
	e.logWriter = w
}

func (e *Event) GetLogWriter() io.Writer {
	return e.logWriter
}

func (e *Event) SetOtherCustomData(ctx context.Context, data interface{}) error {
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": e.ID}, mongoBSON.M{
		"$set": mongoBSON.M{"othercustomdata": data},
	})

	return err
}

func (e *Event) SetCancelable(ctx context.Context, cancelable bool) error {
	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": e.ID}, mongoBSON.M{
		"$set": mongoBSON.M{"cancelable": cancelable},
	})

	return err
}

func (e *Event) Logf(format string, params ...interface{}) {
	log.Debugf(fmt.Sprintf("%s(%s)[%s] %s", e.Target.Type, e.Target.Value, e.Kind, format), params...)
	format += "\n"
	fmt.Fprintf(e, format, params...)
}

func (e *Event) Write(data []byte) (int, error) {
	if e.logWriter != nil {
		e.logWriter.Write(data)
	}
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.StructuredLog = append(e.StructuredLog, eventTypes.LogEntry{
		Date:    time.Now().UTC(),
		Message: string(data),
	})
	return len(data), nil
}

func (e *Event) CancelableContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	if e == nil || !e.Cancelable {
		return ctx, cancel
	}
	wg := sync.WaitGroup{}
	cancelWrapper := func() {
		cancel()
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			canceled, err := e.AckCancel(context.Background())
			if err != nil {
				log.Errorf("unable to check if event was canceled: %v", err)
				continue
			}
			if canceled {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	return ctx, cancelWrapper
}

func (e *Event) TryCancel(ctx context.Context, reason, owner string) error {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if !e.Cancelable || !e.Running {
		return ErrNotCancelable
	}

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}

	update := mongoBSON.M{"$set": mongoBSON.M{
		"cancelinfo": eventTypes.CancelInfo{
			Owner:     owner,
			Reason:    reason,
			StartTime: time.Now().UTC(),
			Asked:     true,
		},
	}}
	query := mongoBSON.M{"_id": e.ID, "cancelinfo.asked": false}
	options := options.FindOneAndUpdate().SetReturnDocument(options.After)

	err = collection.FindOneAndUpdate(ctx, query, update, options).Decode(&e.EventData)

	if err == mongo.ErrNoDocuments {
		if _, errID := GetByID(ctx, e.ID); errID == ErrEventNotFound {
			return ErrEventNotFound
		}
		err = ErrCancelAlreadyRequested
	}
	return err
}

func (e *Event) AckCancel(ctx context.Context) (bool, error) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if !e.Cancelable || !e.Running {
		return false, nil
	}

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return false, err
	}

	update := mongoBSON.M{"$set": mongoBSON.M{
		"cancelinfo.acktime":  time.Now().UTC(),
		"cancelinfo.canceled": true,
	}}
	query := mongoBSON.M{"_id": e.ID, "cancelinfo.asked": true}
	options := options.FindOneAndUpdate().SetReturnDocument(options.After)

	err = collection.FindOneAndUpdate(ctx, query, update, options).Decode(&e.EventData)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return err == nil, err
}

func (e *Event) StartData(value interface{}) error {
	if e.StartCustomData.Type == 0 {
		return nil
	}
	return e.StartCustomData.Unmarshal(value)
}

func (e *Event) EndData(value interface{}) error {
	if e.EndCustomData.Type == 0 {
		return nil
	}
	return e.EndCustomData.Unmarshal(value)
}

func (e *Event) OtherData(value interface{}) error {
	if e.OtherCustomData.Type == 0 {
		return nil
	}
	return e.OtherCustomData.Unmarshal(value)
}

func (e *Event) done(ctx context.Context, evtErr error, customData interface{}, abort bool) (err error) {
	ctx = context.WithoutCancel(ctx)
	// Done will be usually called in a defer block ignoring errors. This is
	// why we log error messages here.
	defer func() {
		e.fillLegacyLog()
		eventDuration.WithLabelValues(e.Kind.Name).Observe(time.Since(e.StartTime).Seconds())
		eventCurrent.WithLabelValues(e.Kind.Name).Dec()
		if err != nil {
			log.Errorf("[events] error marking event as done - %#v: %s", e, err)
		} else {
			if !abort && servicemanager.Webhook != nil {
				servicemanager.Webhook.Notify(ctx, e.ID.Hex())
			}
		}
	}()
	updater.remove(e.ID)

	collection, err := storagev2.EventsCollection()
	if err != nil {
		return err
	}
	if abort {
		_, err = collection.DeleteOne(ctx, mongoBSON.M{"_id": e.ID})
		return err

	}
	if evtErr != nil {
		if errors.Cause(evtErr) == context.Canceled && !e.CancelInfo.Canceled {
			now := time.Now().UTC()
			e.CancelInfo = eventTypes.CancelInfo{
				Owner:     e.Owner.String(),
				Reason:    context.Canceled.Error(),
				AckTime:   now,
				StartTime: now,
				Canceled:  true,
			}
		}
		e.Error = evtErr.Error()
	} else if e.CancelInfo.Canceled {
		e.Error = "canceled by user request"
	}
	e.EndTime = time.Now().UTC()
	e.EndCustomData, err = makeBSONRaw(customData)
	if err != nil {
		return err
	}
	e.Running = false
	var dbEvt Event
	err = collection.FindOne(ctx, mongoBSON.M{"_id": e.ID}).Decode(&dbEvt.EventData)
	if err == nil {
		e.OtherCustomData = dbEvt.OtherCustomData
	}
	e.logMu.Lock()
	defer e.logMu.Unlock()

	if e.EventData.Lock != nil {
		e.EventData.Lock = nil
	}

	_, err = collection.UpdateOne(ctx, mongoBSON.M{"_id": e.ID}, mongoBSON.M{"$set": e.EventData, "$unset": mongoBSON.M{"lock": ""}})

	return err
}

func (e *Event) Log() string {
	if len(e.StructuredLog) == 0 {
		return e.EventData.Log
	}
	msgs := make([]string, len(e.StructuredLog))
	for i, entry := range e.StructuredLog {
		if entry.Date.IsZero() {
			msgs[i] = entry.Message
			continue
		}
		msgs[i] = addLinePrefix(entry.Message, entry.Date.Local().Format(timeFormat)+": ")
	}
	return strings.Join(msgs, "")
}

func (e *Event) fillLegacyLog() {
	if e.EventData.Log != "" || len(e.StructuredLog) == 0 {
		return
	}
	e.EventData.Log = e.Log()
}

func (e *Event) Clone() *Event {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e2 := Event{
		EventData: e.EventData,
		logWriter: e.logWriter,
	}
	e2.EventData.StructuredLog = nil
	return &e2
}

func (e *Event) LogsFrom(origin *Event) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	origin.logMu.Lock()
	defer origin.logMu.Unlock()
	e.StructuredLog = append(e.StructuredLog, origin.StructuredLog...)
}

func (e *Event) OwnerEmail() string {
	switch e.Owner.Type {
	case eventTypes.OwnerTypeUser:
		return e.Owner.Name

	case eventTypes.OwnerTypeToken:
		return fmt.Sprintf("%s@%s", e.Owner.Name, authTypes.TsuruTokenEmailDomain)

	default:
		return ""
	}

}

func checkIsExpired(ctx context.Context, collection *mongo.Collection, lock interface{}) bool {
	var existingEvt Event

	err := collection.FindOne(ctx, mongoBSON.M{"lock": lock}).Decode(&existingEvt.EventData)

	if err == nil {
		now := time.Now().UTC()
		lastUpdate := existingEvt.LockUpdateTime.UTC()
		if now.After(lastUpdate.Add(lockExpireTimeout)) {
			existingEvt.Done(context.TODO(), errors.Errorf("event expired, no update for %v", time.Since(lastUpdate)))
			return true
		}
	}
	return false
}

func FormToCustomData(form url.Values) []map[string]interface{} {
	ret := make([]map[string]interface{}, 0, len(form))
	for k, v := range form {
		var val interface{} = v
		if len(v) == 1 {
			val = v[0]
		}
		ret = append(ret, map[string]interface{}{"name": k, "value": val})
	}
	return ret
}

func addLinePrefix(data string, prefix string) string {
	suffix := ""
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
		suffix = "\n"
	}
	replacement := "\n" + prefix
	return prefix + strings.ReplaceAll(data, "\n", replacement) + suffix
}
