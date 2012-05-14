package gitosis

const (
	AddKey = iota
	RemoveKey
	AddMember
	RemoveMember
	AddGroup
)

// Change encapsulates a change that will be requested to the gitosis file.
//
// The kind is an integer, but you should not send a magic number. Try sending
// one of the package's constant, and Args represent the args for the kind of
// change. If the change provide any response, it will be sent though the
// response channel (a string channel). Example:
//
//     args := map[string]string{
//         "key":    "my-key",
//         "member": "chico",
//     }
//     change := Change{Kind: AddKey, Args: args}
//
// The change in the code above says:
//
//     "add the key my-key to the member chico"
//
// For this kind of change, the key file name will be sent in the channel
// Response.
type Change struct {
	Kind     int
	Args     map[string]string
	Response chan string
}

var Changes = make(chan Change)

func init() {
	go processChanges()
}

func processChanges() {
	for change := range Changes {
		switch change.Kind {
		case AddKey:
			go func(ch chan string) {
				keyfile, _ := buildAndStoreKeyFile(change.Args["member"], change.Args["key"])
				ch <- keyfile
			}(change.Response)
		case RemoveKey:
			go deleteKeyFile(change.Args["key"])
		case AddMember:
			go addMember(change.Args["group"], change.Args["member"])
		case RemoveMember:
			go removeMember(change.Args["group"], change.Args["member"])
		case AddGroup:
			go addGroup(change.Args["group"])
		}
	}
}
