package registry

import "github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/log"

// NopRegistry is an implementation of Registry for applications that do not need service discovery.
type NopRegistry struct {
}

// RegisterApp is a no-op.
func (s *NopRegistry) RegisterApp(*AppRegistration) error {
	log.Infof("Skipping application registration for NopRegistry")
	return nil
}

// RegisterHandler is a no-op.
func (s *NopRegistry) RegisterHandler(*HandlerRegistration) error {
	log.Infof("Skipping handler registration for NopRegistry")
	return nil
}
