package provision

import "io"

type Deployer interface {
	Deploy(App, io.Writer) error
}
