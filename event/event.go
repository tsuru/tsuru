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
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	internalConfig "github.com/tsuru/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/types/tracker"
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
	ErrInvalidTargetType      = errors.New("invalid event target type")

	OwnerTypeUser     = ownerType("user")
	OwnerTypeApp      = ownerType("app")
	OwnerTypeInternal = ownerType("internal")
	OwnerTypeToken    = ownerType("token")

	KindTypePermission = kindType("permission")
	KindTypeInternal   = kindType("internal")

	TargetTypeGlobal          = TargetType("global")
	TargetTypeApp             = TargetType("app")
	TargetTypeNode            = TargetType("node")
	TargetTypeContainer       = TargetType("container")
	TargetTypePool            = TargetType("pool")
	TargetTypeService         = TargetType("service")
	TargetTypeServiceInstance = TargetType("service-instance")
	TargetTypeServiceBroker   = TargetType("service-broker")
	TargetTypeTeam            = TargetType("team")
	TargetTypeUser            = TargetType("user")
	TargetTypeIaas            = TargetType("iaas")
	TargetTypeRole            = TargetType("role")
	TargetTypePlatform        = TargetType("platform")
	TargetTypePlan            = TargetType("plan")
	TargetTypeNodeContainer   = TargetType("node-container")
	TargetTypeInstallHost     = TargetType("install-host")
	TargetTypeEventBlock      = TargetType("event-block")
	TargetTypeCluster         = TargetType("cluster")
	TargetTypeVolume          = TargetType("volume")
	TargetTypeWebhook         = TargetType("webhook")
	TargetTypeGC              = TargetType("gc")
	TargetTypeRouter          = TargetType("router")
)

const (
	filterMaxLimit = 100
)

func init() {
	prometheus.MustRegister(eventDuration, eventCurrent, eventsRejected)
}

type ErrThrottled struct {
	Spec       *ThrottlingSpec
	Target     Target
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

type Target struct {
	Type  TargetType
	Value string
}

func (id Target) GetBSON() (interface{}, error) {
	return bson.D{{Name: "type", Value: id.Type}, {Name: "value", Value: id.Value}}, nil
}

func (id Target) IsValid() bool {
	return id.Type != ""
}

func (id Target) String() string {
	return fmt.Sprintf("%s(%s)", id.Type, id.Value)
}

type eventID struct {
	Target Target
	ObjId  bson.ObjectId
}

func (id *eventID) SetBSON(raw bson.Raw) error {
	err := raw.Unmarshal(&id.Target)
	if err != nil {
		return raw.Unmarshal(&id.ObjId)
	}
	return nil
}

func (id eventID) GetBSON() (interface{}, error) {
	if len(id.ObjId) != 0 {
		return id.ObjId, nil
	}
	return id.Target.GetBSON()
}

// This private type allow us to export the main Event struct without allowing
// access to its public fields. (They have to be public for database
// serializing).
type eventData struct {
	ID              eventID `bson:"_id"`
	UniqueID        bson.ObjectId
	StartTime       time.Time
	EndTime         time.Time     `bson:",omitempty"`
	Target          Target        `bson:",omitempty"`
	ExtraTargets    []ExtraTarget `bson:",omitempty"`
	StartCustomData bson.Raw      `bson:",omitempty"`
	EndCustomData   bson.Raw      `bson:",omitempty"`
	OtherCustomData bson.Raw      `bson:",omitempty"`
	Kind            Kind
	Owner           Owner
	SourceIP        string
	LockUpdateTime  time.Time
	Error           string
	Log             string     `bson:",omitempty"`
	StructuredLog   []LogEntry `bson:",omitempty"`
	CancelInfo      cancelInfo
	Cancelable      bool
	Running         bool
	Allowed         AllowedPermission
	AllowedCancel   AllowedPermission
	Instance        tracker.TrackedInstance
}

type LogEntry struct {
	Date    time.Time
	Message string
}

type cancelInfo struct {
	Owner     string
	StartTime time.Time
	AckTime   time.Time
	Reason    string
	Asked     bool
	Canceled  bool
}

type ownerType string

type kindType string

type TargetType string

func GetTargetType(t string) (TargetType, error) {
	switch t {
	case "global":
		return TargetTypeGlobal, nil
	case "app":
		return TargetTypeApp, nil
	case "node":
		return TargetTypeNode, nil
	case "container":
		return TargetTypeContainer, nil
	case "pool":
		return TargetTypePool, nil
	case "service":
		return TargetTypeService, nil
	case "service-instance":
		return TargetTypeServiceInstance, nil
	case "team":
		return TargetTypeTeam, nil
	case "user":
		return TargetTypeUser, nil
	case "iaas":
		return TargetTypeIaas, nil
	case "role":
		return TargetTypeRole, nil
	case "platform":
		return TargetTypePlatform, nil
	case "plan":
		return TargetTypePlan, nil
	case "node-container":
		return TargetTypeNodeContainer, nil
	case "install-host":
		return TargetTypeInstallHost, nil
	case "event-block":
		return TargetTypeEventBlock, nil
	case "cluster":
		return TargetTypeCluster, nil
	case "volume":
		return TargetTypeVolume, nil
	case "webhook":
		return TargetTypeWebhook, nil
	case "router":
		return TargetTypeRouter, nil
	}
	return TargetType(""), ErrInvalidTargetType
}

type Owner struct {
	Type ownerType
	Name string
}

type Kind struct {
	Type kindType
	Name string
}

func (o Owner) String() string {
	return fmt.Sprintf("%s %s", o.Type, o.Name)
}

func (k Kind) String() string {
	return k.Name
}

type ThrottlingSpec struct {
	TargetType TargetType    `json:"target-type"`
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

func throttlingKey(targetType TargetType, kindName string, allTargets bool) string {
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

func getThrottling(t *Target, k *Kind, allTargets bool) *ThrottlingSpec {
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
	eventData
	logMu     sync.Mutex
	logWriter io.Writer
}

type ExtraTarget struct {
	Target Target
	Lock   bool
}

type Opts struct {
	Target        Target
	ExtraTargets  []ExtraTarget
	Kind          *permission.PermissionScheme
	InternalKind  string
	Owner         auth.Token
	RawOwner      Owner
	RemoteAddr    string
	CustomData    interface{}
	DisableLock   bool
	Cancelable    bool
	Allowed       AllowedPermission
	AllowedCancel AllowedPermission
	RetryTimeout  time.Duration
}

func Allowed(scheme *permission.PermissionScheme, contexts ...permTypes.PermissionContext) AllowedPermission {
	return AllowedPermission{
		Scheme:   scheme.FullName(),
		Contexts: contexts,
	}
}

type AllowedPermission struct {
	Scheme   string
	Contexts []permTypes.PermissionContext `bson:",omitempty"`
}

func (ap *AllowedPermission) GetBSON() (interface{}, error) {
	var ctxs []bson.D
	for _, ctx := range ap.Contexts {
		ctxs = append(ctxs, bson.D{
			{Name: "ctxtype", Value: ctx.CtxType},
			{Name: "value", Value: ctx.Value},
		})
	}
	return bson.M{
		"scheme":   ap.Scheme,
		"contexts": ctxs,
	}, nil
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
	Type   TargetType
	Values []string
}

type Filter struct {
	Target         Target
	KindType       kindType
	KindNames      []string `form:"-"`
	OwnerType      ownerType
	OwnerName      string
	Since          time.Time
	Until          time.Time
	Running        *bool
	ErrorOnly      bool
	Raw            bson.M
	AllowedTargets []TargetFilter
	Permissions    []permission.Permission

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

func (f *Filter) toQuery() (bson.M, error) {
	query := bson.M{}
	permMap := map[string][]permTypes.PermissionContext{}
	andBlock := []bson.M{}
	if f.Permissions != nil {
		for _, p := range f.Permissions {
			permMap[p.Scheme.FullName()] = append(permMap[p.Scheme.FullName()], p.Context)
		}
		var permOrBlock []bson.M
		for perm, ctxs := range permMap {
			ctxsBson := []bson.D{}
			for _, ctx := range ctxs {
				if ctx.CtxType == permTypes.CtxGlobal {
					ctxsBson = nil
					break
				}
				ctxsBson = append(ctxsBson, bson.D{
					{Name: "ctxtype", Value: ctx.CtxType},
					{Name: "value", Value: ctx.Value},
				})
			}
			toAppend := bson.M{
				"allowed.scheme": bson.M{"$regex": "^" + strings.Replace(perm, ".", `\.`, -1)},
			}
			if ctxsBson != nil {
				toAppend["allowed.contexts"] = bson.M{"$in": ctxsBson}
			}
			permOrBlock = append(permOrBlock, toAppend)
		}
		andBlock = append(andBlock, bson.M{"$or": permOrBlock})
	}
	if f.AllowedTargets != nil {
		var orBlock []bson.M
		for _, at := range f.AllowedTargets {
			f := bson.M{"target.type": at.Type}
			extraF := bson.M{"extratargets.target.type": at.Type}
			if at.Values != nil {
				f["target.value"] = bson.M{"$in": at.Values}
				extraF["extratargets.target.value"] = bson.M{"$in": at.Values}
			}
			orBlock = append(orBlock, f, extraF)
		}
		if len(orBlock) == 0 {
			return nil, errInvalidQuery
		}
		andBlock = append(andBlock, bson.M{"$or": orBlock})
	}
	if f.Target.Type != "" {
		orBlock := []bson.M{
			{"target.type": f.Target.Type},
			{"extratargets.target.type": f.Target.Type},
		}
		andBlock = append(andBlock, bson.M{"$or": orBlock})
	}
	if f.Target.Value != "" {
		orBlock := []bson.M{
			{"target.value": f.Target.Value},
			{"extratargets.target.value": f.Target.Value},
		}
		andBlock = append(andBlock, bson.M{"$or": orBlock})
	}
	if f.KindType != "" {
		query["kind.type"] = f.KindType
	}
	if len(f.KindNames) > 0 {
		query["kind.name"] = bson.M{"$in": f.KindNames}
	}
	if f.OwnerType != "" {
		query["owner.type"] = f.OwnerType
	}
	if f.OwnerName != "" {
		query["owner.name"] = f.OwnerName
	}
	var timeParts []bson.M
	if !f.Since.IsZero() {
		timeParts = append(timeParts, bson.M{"starttime": bson.M{"$gte": f.Since}})
	}
	if !f.Until.IsZero() {
		timeParts = append(timeParts, bson.M{"starttime": bson.M{"$lte": f.Until}})
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
		query["error"] = bson.M{"$ne": ""}
	}
	if f.Raw != nil {
		for k, v := range f.Raw {
			query[k] = v
		}
	}
	return query, nil
}

func GetKinds() ([]Kind, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	var kinds []Kind
	err = coll.Find(nil).Distinct("kind", &kinds)
	if err != nil {
		return nil, err
	}
	return kinds, nil
}

func transformEvent(data eventData) *Event {
	var event Event
	event.eventData = data
	event.fillLegacyLog()
	return &event
}

func GetRunning(target Target, kind string) (*Event, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	var evtData eventData
	err = coll.Find(bson.M{
		"_id":       eventID{Target: target},
		"kind.name": kind,
		"running":   true,
	}).One(&evtData)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	evt := transformEvent(evtData)
	return evt, nil
}

func GetByHexID(hexid string) (*Event, error) {
	if !bson.IsObjectIdHex(hexid) {
		return nil, errors.Errorf("receive ID is not a valid event object id: %q", hexid)
	}
	return GetByID(bson.ObjectIdHex(hexid))
}

func GetByID(id bson.ObjectId) (*Event, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	var evtData eventData
	err = coll.Find(bson.M{
		"uniqueid": id,
	}).One(&evtData)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	evt := transformEvent(evtData)
	return evt, nil
}

func All() ([]*Event, error) {
	return List(nil)
}

func List(filter *Filter) ([]*Event, error) {
	limit := 0
	skip := 0
	var query bson.M
	var err error
	sort := "-starttime"
	if filter != nil {
		limit = filterMaxLimit
		if filter.Limit != 0 {
			limit = filter.Limit
		}
		if filter.Sort != "" {
			sort = filter.Sort
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
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	find := coll.Find(query).Sort(sort)
	if limit > 0 {
		find = find.Limit(limit)
	}
	if skip > 0 {
		find = find.Skip(skip)
	}
	var allData []eventData
	err = find.All(&allData)
	if err != nil {
		return nil, err
	}
	evts := make([]*Event, len(allData))
	for i := range evts {
		evts[i] = transformEvent(allData[i])
	}
	return evts, nil
}

func New(opts *Opts) (*Event, error) {
	if opts == nil {
		return nil, ErrNoOpts
	}
	if opts.Owner == nil && opts.RawOwner.Name == "" && opts.RawOwner.Type == "" {
		return nil, ErrNoOwner
	}
	if opts.Kind == nil {
		return nil, ErrNoKind
	}
	if opts.RetryTimeout == 0 && opts.Target.Type == TargetTypeApp {
		opts.RetryTimeout = defaultAppRetryTimeout
	}
	return newEvt(opts)
}

func NewInternal(opts *Opts) (*Event, error) {
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
	return newEvt(opts)
}

func NewInternalMany(targets []Target, opts *Opts) (*Event, error) {
	if len(targets) == 0 {
		return nil, errors.New("event must have at least one target")
	}
	opts.Target = targets[0]
	for _, target := range targets[1:] {
		opts.ExtraTargets = append(opts.ExtraTargets, ExtraTarget{
			Target: target,
			Lock:   true,
		})
	}
	return NewInternal(opts)
}

func makeBSONRaw(in interface{}) (bson.Raw, error) {
	if in == nil {
		return bson.Raw{}, nil
	}
	var kind byte
	v := reflect.ValueOf(in)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return bson.Raw{}, nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Map, reflect.Struct:
		kind = 3 // BSON "Document" kind
	case reflect.Array, reflect.Slice:
		kind = 4 // BSON "Array" kind
	default:
		return bson.Raw{}, errors.Errorf("cannot use type %T as event custom data", in)
	}
	data, err := bson.Marshal(in)
	if err != nil {
		return bson.Raw{}, err
	}
	if len(data) == 0 {
		return bson.Raw{}, errors.Errorf("invalid empty bson object for object %#v", in)
	}
	return bson.Raw{
		Kind: kind,
		Data: data,
	}, nil
}

func checkThrottling(coll *storage.Collection, target *Target, kind *Kind, allTargets bool) error {
	tSpec := getThrottling(target, kind, allTargets)
	if tSpec == nil || tSpec.Max <= 0 || tSpec.Time <= 0 {
		return nil
	}
	query := bson.M{
		"target.type": target.Type,
	}
	now := time.Now().UTC()
	startTimeQuery := bson.M{"$gt": now.Add(-tSpec.Time)}
	if tSpec.WaitFinish {
		query["$or"] = []bson.M{
			{"starttime": startTimeQuery},
			{
				"running":        true,
				"lockupdatetime": bson.M{"$gt": now.Add(-lockExpireTimeout)},
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
	c, err := coll.Find(query).Count()
	if err != nil {
		return err
	}
	if c >= tSpec.Max {
		return ErrThrottled{Spec: tSpec, Target: *target, AllTargets: allTargets}
	}
	return nil
}

func newEvt(opts *Opts) (evt *Event, err error) {
	if opts.RetryTimeout == 0 {
		return newEvtOnce(opts)
	}

	timeoutCh := time.After(opts.RetryTimeout)
	for {
		evt, err := newEvtOnce(opts)
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

func newEvtOnce(opts *Opts) (evt *Event, err error) {
	var k Kind
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
		k.Type = KindTypeInternal
		k.Name = opts.InternalKind
	} else {
		k.Type = KindTypePermission
		k.Name = opts.Kind.FullName()
	}
	var o Owner
	if opts.Owner == nil {
		if opts.RawOwner.Name != "" && opts.RawOwner.Type != "" {
			o = opts.RawOwner
		} else {
			o.Type = OwnerTypeInternal
		}
	} else {
		if token, ok := opts.Owner.(authTypes.NamedToken); ok {
			o.Type = OwnerTypeToken
			o.Name = token.GetTokenName()
		} else if opts.Owner.IsAppToken() {
			o.Type = OwnerTypeApp
			o.Name = opts.Owner.GetAppName()
		} else {
			o.Type = OwnerTypeUser
			o.Name = opts.Owner.GetUserName()
		}
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	err = checkThrottling(coll, &opts.Target, &k, false)
	if err != nil {
		return nil, err
	}
	err = checkThrottling(coll, &opts.Target, &k, true)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	raw, err := makeBSONRaw(opts.CustomData)
	if err != nil {
		return nil, err
	}
	uniqID := bson.NewObjectId()
	var id eventID
	if opts.DisableLock {
		id.ObjId = uniqID
	} else {
		id.Target = opts.Target
	}
	instance, err := servicemanager.InstanceTracker.CurrentInstance(context.TODO())
	if err != nil {
		return nil, err
	}

	sourceIP := ""
	if opts.RemoteAddr != "" {
		sourceIP, _, _ = net.SplitHostPort(opts.RemoteAddr)
	}

	evt = &Event{eventData: eventData{
		ID:              id,
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
	}}
	maxRetries := 1
	for i := 0; i < maxRetries+1; i++ {
		err = coll.Insert(evt.eventData)
		if err == nil {
			err = checkLocked(evt, opts.DisableLock)
			if err != nil {
				evt.Abort()
				return nil, err
			}
			err = checkIsBlocked(evt)
			if err != nil {
				evt.Done(err)
				return nil, err
			}
			updater.add(id)
			return evt, nil
		}
		if mgo.IsDup(err) {
			if i >= maxRetries || !checkIsExpired(coll, evt.ID) {
				var existing Event
				err = coll.FindId(evt.ID).One(&existing.eventData)
				if err == mgo.ErrNotFound {
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

func checkLocked(evt *Event, disableLock bool) error {
	var targets []Target
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
	var orBlock []bson.M
	for _, t := range targets {
		tBson, _ := t.GetBSON()
		orBlock = append(orBlock, bson.M{"_id": tBson}, bson.M{
			"extratargets": bson.M{"$elemMatch": bson.M{"target": tBson, "lock": true}},
		})
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	var existing Event
	err = coll.Find(bson.M{
		"running":  true,
		"uniqueid": bson.M{"$ne": evt.UniqueID},
		"$or":      orBlock,
	}).One(&existing.eventData)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return err
	}
	return ErrEventLocked{Event: &existing}
}

func (e *Event) RawInsert(start, other, end interface{}) error {
	e.ID = eventID{ObjId: e.UniqueID}
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
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	e.logMu.Lock()
	defer e.logMu.Unlock()
	return coll.Insert(e.eventData)
}

func (e *Event) Abort() error {
	return e.done(nil, nil, true)
}

func (e *Event) Done(evtErr error) error {
	return e.done(evtErr, nil, false)
}

func (e *Event) DoneCustomData(evtErr error, customData interface{}) error {
	return e.done(evtErr, customData, false)
}

func (e *Event) SetLogWriter(w io.Writer) {
	e.logWriter = w
}

func (e *Event) GetLogWriter() io.Writer {
	return e.logWriter
}

func (e *Event) SetOtherCustomData(data interface{}) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	return coll.UpdateId(e.ID, bson.M{
		"$set": bson.M{"othercustomdata": data},
	})
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
	e.StructuredLog = append(e.StructuredLog, LogEntry{
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
			canceled, err := e.AckCancel()
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

func (e *Event) TryCancel(reason, owner string) error {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if !e.Cancelable || !e.Running {
		return ErrNotCancelable
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"cancelinfo": cancelInfo{
				Owner:     owner,
				Reason:    reason,
				StartTime: time.Now().UTC(),
				Asked:     true,
			},
		}},
		ReturnNew: true,
	}
	_, err = coll.Find(bson.M{"_id": e.ID, "cancelinfo.asked": false}).Apply(change, &e.eventData)
	if err == mgo.ErrNotFound {
		if _, errID := GetByID(e.UniqueID); errID == ErrEventNotFound {
			return ErrEventNotFound
		}
		err = ErrCancelAlreadyRequested
	}
	return err
}

func (e *Event) AckCancel() (bool, error) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if !e.Cancelable || !e.Running {
		return false, nil
	}
	conn, err := db.Conn()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	coll := conn.Events()
	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"cancelinfo.acktime":  time.Now().UTC(),
			"cancelinfo.canceled": true,
		}},
		ReturnNew: true,
	}
	_, err = coll.Find(bson.M{"_id": e.ID, "cancelinfo.asked": true}).Apply(change, &e.eventData)
	if err == mgo.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}

func (e *Event) StartData(value interface{}) error {
	if e.StartCustomData.Kind == 0 {
		return nil
	}
	return e.StartCustomData.Unmarshal(value)
}

func (e *Event) EndData(value interface{}) error {
	if e.EndCustomData.Kind == 0 {
		return nil
	}
	return e.EndCustomData.Unmarshal(value)
}

func (e *Event) OtherData(value interface{}) error {
	if e.OtherCustomData.Kind == 0 {
		return nil
	}
	return e.OtherCustomData.Unmarshal(value)
}

func (e *Event) done(evtErr error, customData interface{}, abort bool) (err error) {
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
				servicemanager.Webhook.Notify(e.UniqueID.Hex())
			}
		}
	}()
	updater.remove(e.ID)
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	if abort {
		return coll.RemoveId(e.ID)
	}
	if evtErr != nil {
		if errors.Cause(evtErr) == context.Canceled && !e.CancelInfo.Canceled {
			now := time.Now().UTC()
			e.CancelInfo = cancelInfo{
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
	err = coll.FindId(e.ID).One(&dbEvt.eventData)
	if err == nil {
		e.OtherCustomData = dbEvt.OtherCustomData
	}
	e.logMu.Lock()
	defer e.logMu.Unlock()
	if len(e.ID.ObjId) != 0 {
		return coll.UpdateId(e.ID, e.eventData)
	}
	defer coll.RemoveId(e.ID)
	e.ID = eventID{ObjId: e.UniqueID}
	return coll.Insert(e.eventData)
}

func (e *Event) Log() string {
	if len(e.StructuredLog) == 0 {
		return e.eventData.Log
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
	if e.eventData.Log != "" || len(e.StructuredLog) == 0 {
		return
	}
	e.eventData.Log = e.Log()
}

func (e *Event) Clone() *Event {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e2 := Event{
		eventData: e.eventData,
		logWriter: e.logWriter,
	}
	e2.eventData.StructuredLog = nil
	return &e2
}

func (e *Event) LogsFrom(origin *Event) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	origin.logMu.Lock()
	defer origin.logMu.Unlock()
	e.StructuredLog = append(e.StructuredLog, origin.StructuredLog...)
}

func checkIsExpired(coll *storage.Collection, id interface{}) bool {
	var existingEvt Event
	err := coll.FindId(id).One(&existingEvt.eventData)
	if err == nil {
		now := time.Now().UTC()
		lastUpdate := existingEvt.LockUpdateTime.UTC()
		if now.After(lastUpdate.Add(lockExpireTimeout)) {
			existingEvt.Done(errors.Errorf("event expired, no update for %v", time.Since(lastUpdate)))
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

func Migrate(query bson.M, cb func(*Event) error) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	iter := coll.Find(query).Iter()
	var evtData eventData
	for iter.Next(&evtData) {
		evt := &Event{eventData: evtData}
		err = cb(evt)
		if err != nil {
			return errors.Wrapf(err, "unable to migrate %#v", evt)
		}
		err = coll.UpdateId(evt.ID, evt.eventData)
		if err != nil {
			return errors.Wrapf(err, "unable to update %#v", evt)
		}
	}
	return iter.Close()
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
