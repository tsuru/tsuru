package scroll

type Scope int

const (
	ScopePublic Scope = iota
	ScopeProtected
)

var scopes = []string{
	"public",
	"protected",
}

func (scope Scope) String() string {
	return scopes[scope]
}
