package event

import (
	"fmt"
	"sync"
	"time"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	lockUpdateInterval = 30 * time.Second
	lockExpireTimeout  = 5 * time.Minute
	updater            = lockUpdater{
		addCh:    make(chan *EventTarget),
		removeCh: make(chan *EventTarget),
		stopCh:   make(chan struct{}),
	}
)

type ErrEventLocked struct{ event *Event }

func (err ErrEventLocked) Error() string {
	return fmt.Sprintf("event locked: %v", err.event)
}

type EventTarget struct{ Name, Value string }

func (id EventTarget) GetBSON() (interface{}, error) {
	return bson.D{{"name", id.Name}, {"value", id.Value}}, nil
}

func EventTargetFactory(name string) func(string) EventTarget {
	return func(value string) EventTarget {
		return EventTarget{Name: name, Value: value}
	}
}

type eventId struct {
	target EventTarget
	objId  bson.ObjectId
}

func (id *eventId) SetBSON(raw bson.Raw) error {
	err := raw.Unmarshal(&id.target)
	if err != nil {
		return raw.Unmarshal(&id.objId)
	}
	return nil
}

func (id eventId) GetBSON() (interface{}, error) {
	if len(id.objId) != 0 {
		return id.objId, nil
	}
	return id.target.GetBSON()
}

// This private type allow us to export the main Event struct without allowing
// access to its public fields. (They have to be public for database
// serializing).
type eventData struct {
	ID              eventId `bson:"_id"`
	StartTime       time.Time
	EndTime         time.Time   `bson:",omitempty"`
	Target          EventTarget `bson:",omitempty"`
	StartCustomData interface{} `bson:",omitempty"`
	EndCustomData   interface{} `bson:",omitempty"`
	Kind            string
	Owner           string
	Cancelable      bool
	Running         bool
	LockUpdateTime  time.Time
	Error           string
}

type Event struct {
	eventData
}

func (e *Event) String() string {
	return fmt.Sprintf("%s(%s) running %q start by %s at %s",
		e.Target.Name,
		e.Target.Value,
		e.Kind,
		e.Owner,
		e.StartTime.Format(time.RFC3339),
	)
}

func AllEvents() ([]Event, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var allData []eventData
	err = conn.Events().Find(nil).Sort("-_id").All(&allData)
	evts := make([]Event, len(allData))
	for i := range evts {
		evts[i].eventData = allData[i]
	}
	return evts, err
}

func NewEvent(target EventTarget, kind, owner string) (*Event, error) {
	return NewEventCustomData(target, kind, owner, nil)
}

func NewEventCustomData(target EventTarget, kind, owner string, customData interface{}) (*Event, error) {
	updater.start()
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	coll := conn.Events()
	now := time.Now().UTC()
	evt := Event{eventData{
		ID:              eventId{target: target},
		Target:          target,
		StartTime:       now,
		Kind:            kind,
		Owner:           owner,
		StartCustomData: customData,
		LockUpdateTime:  now,
		Running:         true,
	}}
	maxRetries := 1
	for i := 0; i < maxRetries+1; i++ {
		err = coll.Insert(evt.eventData)
		if err == nil {
			updater.addCh <- &target
			return &evt, nil
		}
		if mgo.IsDup(err) {
			if i >= maxRetries || !checkIsExpired(coll, evt.ID) {
				var existing Event
				err = coll.FindId(evt.ID).One(&existing.eventData)
				if err == nil {
					err = ErrEventLocked{event: &existing}
				}
			}
		}
	}
	return nil, err
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

func (e *Event) done(evtErr error, customData interface{}, abort bool) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	coll := conn.Events()
	updater.removeCh <- &e.Target
	if abort {
		return coll.RemoveId(e.ID)
	}
	if evtErr != nil {
		e.Error = evtErr.Error()
	}
	e.EndTime = time.Now().UTC()
	e.EndCustomData = customData
	e.Running = false
	defer coll.RemoveId(e.ID)
	e.ID = eventId{objId: bson.NewObjectId()}
	return coll.Insert(e.eventData)
}

type lockUpdater struct {
	addCh    chan *EventTarget
	removeCh chan *EventTarget
	stopCh   chan struct{}
	once     sync.Once
}

func (l *lockUpdater) start() {
	l.once.Do(func() { go l.spin() })
}

func (l *lockUpdater) stop() {
	l.stopCh <- struct{}{}
	l.once = sync.Once{}
}

func (l *lockUpdater) spin() {
	set := map[EventTarget]struct{}{}
	for {
		select {
		case added := <-l.addCh:
			set[*added] = struct{}{}
		case removed := <-l.removeCh:
			delete(set, *removed)
		case <-l.stopCh:
			return
		case <-time.After(lockUpdateInterval):
		}
		conn, err := db.Conn()
		if err != nil {
			log.Errorf("[event lock update] error getting db conn: %s", err)
			continue
		}
		coll := conn.Events()
		slice := make([]interface{}, len(set))
		i := 0
		for id := range set {
			slice[i], _ = id.GetBSON()
			i++
		}
		err = coll.Update(bson.M{"_id": bson.M{"$in": slice}}, bson.M{"$set": bson.M{"lockupdatetime": time.Now().UTC()}})
		if err != nil {
			log.Errorf("[event lock update] error updating: %s", err)
		}
		conn.Close()
	}
}

func checkIsExpired(coll *storage.Collection, id interface{}) bool {
	var existingEvt Event
	err := coll.FindId(id).One(&existingEvt.eventData)
	if err == nil {
		now := time.Now().UTC()
		lastUpdate := existingEvt.LockUpdateTime.UTC()
		if now.After(lastUpdate.Add(lockExpireTimeout)) {
			existingEvt.Done(fmt.Errorf("event expired, no update for %v", time.Since(lastUpdate)))
			return true
		}
	}
	return false
}
