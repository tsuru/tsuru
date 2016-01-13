package middleware

import "fmt"

const (
	RewriteType = "rewrite"
	RewriteID   = "rw1"
)

// Rewrite is a spec for the respective vulcan's middleware that enables request/response
// alteration.
type Rewrite struct {
	Regexp      string `json:"Regexp"`
	Replacement string `json:"Replacement"`
	RewriteBody bool   `json:"RewriteBody"`
	Redirect    bool   `json:"Redirect"`
}

func NewRewrite(spec Rewrite) Middleware {
	return Middleware{
		Type:     RewriteType,
		ID:       RewriteID,
		Priority: DefaultPriority,
		Spec:     spec,
	}
}

func (rw Rewrite) String() string {
	return fmt.Sprintf("Rewrite(Regexp=%v, Replacement=%v, RewriteBody=%v, Redirect=%v)",
		rw.Regexp, rw.Replacement, rw.RewriteBody, rw.Redirect)
}
