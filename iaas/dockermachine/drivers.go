// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	cloudstack "github.com/andrestc/docker-machine-driver-cloudstack"
	"github.com/docker/machine/drivers/amazonec2"
	"github.com/docker/machine/drivers/digitalocean"
	"github.com/docker/machine/drivers/exoscale"
	"github.com/docker/machine/drivers/generic"
	"github.com/docker/machine/drivers/google"
	"github.com/docker/machine/drivers/hyperv"
	"github.com/docker/machine/drivers/none"
	"github.com/docker/machine/drivers/openstack"
	"github.com/docker/machine/drivers/rackspace"
	"github.com/docker/machine/drivers/softlayer"
	"github.com/docker/machine/drivers/virtualbox"
	"github.com/docker/machine/drivers/vmwarefusion"
	"github.com/docker/machine/drivers/vmwarevcloudair"
	"github.com/docker/machine/drivers/vmwarevsphere"
	"github.com/docker/machine/libmachine/drivers/plugin"
	"github.com/docker/machine/libmachine/drivers/plugin/localbinary"
	"github.com/pkg/errors"
)

func init() {
	localbinary.CoreDrivers = append(localbinary.CoreDrivers, "cloudstack")
}

func RunDriver(driverName string) error {
	switch driverName {
	case "amazonec2":
		plugin.RegisterDriver(amazonec2.NewDriver("", ""))
	case "digitalocean":
		plugin.RegisterDriver(digitalocean.NewDriver("", ""))
	case "exoscale":
		plugin.RegisterDriver(exoscale.NewDriver("", ""))
	case "generic":
		plugin.RegisterDriver(generic.NewDriver("", ""))
	case "google":
		plugin.RegisterDriver(google.NewDriver("", ""))
	case "hyperv":
		plugin.RegisterDriver(hyperv.NewDriver("", ""))
	case "none":
		plugin.RegisterDriver(none.NewDriver("", ""))
	case "openstack":
		plugin.RegisterDriver(openstack.NewDriver("", ""))
	case "rackspace":
		plugin.RegisterDriver(rackspace.NewDriver("", ""))
	case "softlayer":
		plugin.RegisterDriver(softlayer.NewDriver("", ""))
	case "virtualbox":
		plugin.RegisterDriver(virtualbox.NewDriver("", ""))
	case "vmwarefusion":
		plugin.RegisterDriver(vmwarefusion.NewDriver("", ""))
	case "vmwarevcloudair":
		plugin.RegisterDriver(vmwarevcloudair.NewDriver("", ""))
	case "vmwarevsphere":
		plugin.RegisterDriver(vmwarevsphere.NewDriver("", ""))
	case "cloudstack":
		plugin.RegisterDriver(cloudstack.NewDriver("", ""))
	default:
		return errors.Errorf("Unsupported driver: %s\n", driverName)
	}
	return nil
}

func DefaultParamsForDriver(driverName string) map[string]interface{} {
	params := make(map[string]interface{})
	switch driverName {
	case "amazonec2":
		params["amazonec2-use-private-address"] = true
	case "google":
		params["google-use-internal-ip"] = true
	}
	return params
}
