package middleware

import "fmt"

const (
	ConnLimitType = "connlimit"
	ConnLimitID   = "cl1"
)

// ConnLimit is a spec for the respective vulcan's middleware that lets to control amount if simultaneous
// connections to locations.
type ConnLimit struct {
	Variable    string `json:"Variable"`
	Connections int    `json:"Connections"`
}

func NewConnLimit(spec ConnLimit) Middleware {
	return Middleware{
		Type:     ConnLimitType,
		ID:       ConnLimitID,
		Priority: DefaultPriority,
		Spec:     spec,
	}
}

func (cl ConnLimit) String() string {
	return fmt.Sprintf("ConnLimit(Variable=%v, Connections=%v)",
		cl.Variable, cl.Connections)
}
