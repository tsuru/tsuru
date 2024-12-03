package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
)

func (s *S) TestServiceManagerDeploySimple(c *check.C) {
	type restartStep struct {
		proc    string
		version int
	}
	type stopStep struct {
		proc    string
		version int
	}
	type startStep struct {
		proc    string
		version int
	}
	type unitStep struct {
		proc    string
		version int
		units   int
	}
	type deployStep struct {
		procs            []string
		newVersion       bool
		overrideVersions bool
		routable         bool
	}
	type stepDef struct {
		*restartStep
		*stopStep
		*startStep
		*unitStep
		*deployStep
		check func()
	}

	tests := []struct {
		steps []stepDef
	}{
		{
			steps: []stepDef{
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 1, 1)
						s.hasSvc(c, "myapp0-p1")

						s.noSvc(c, "myapp0-p1-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 2, 1)
						s.hasSvc(c, "myapp0-p1")

						s.noSvc(c, "myapp0-p1-v2")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 2, 1)
						s.hasDepWithVersion(c, "myapp0-p1-v3", 3, 1)
						s.hasSvc(c, "myapp0-p1")
						s.hasSvc(c, "myapp0-p1-v2")
						s.hasSvc(c, "myapp0-p1-v3")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 4, 1)
						s.hasSvc(c, "myapp0-p1")

						s.noDep(c, "myapp0-p1-v3")
						s.noSvc(c, "myapp0-p1-v2")
						s.noSvc(c, "myapp0-p1-v3")
						s.noSvc(c, "myapp0-p1-v4")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p2"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 1)
						s.hasSvc(c, "myapp0-p2")

						s.noDep(c, "myapp0-p1")
						s.noSvc(c, "myapp0-p1")
						s.noSvc(c, "myapp0-p2-v5")
					},
				},
				{
					unitStep: &unitStep{version: 4, units: 2, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 1)
						s.hasSvc(c, "myapp0-p2")
						s.hasDepWithVersion(c, "myapp0-p1-v4", 4, 2)
						s.hasSvc(c, "myapp0-p1-v4")

						s.noDep(c, "myapp0-p1")
						s.noSvc(c, "myapp0-p2-v5")
						s.noSvc(c, "myapp0-p1")
					},
				},
				{
					stopStep: &stopStep{version: 4, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 1)
						s.hasSvc(c, "myapp0-p2")

						s.noDep(c, "myapp0-p1-v4")
						s.noDep(c, "myapp0-p1")
						s.noSvc(c, "myapp0-p1-v4")
						s.noSvc(c, "myapp0-p1")
						s.noSvc(c, "myapp0-p2-v5")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 0)
						s.hasSvc(c, "myapp0-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 1)
						s.hasSvc(c, "myapp0-p2")
					},
				},
				{
					unitStep: &unitStep{version: 5, units: 2, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 3)
						s.hasSvc(c, "myapp0-p2")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 0)
						s.hasSvc(c, "myapp0-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p2", 5, 3)
						s.hasSvc(c, "myapp0-p2")
					},
				},
			},
		},
	}

	for i, tt := range tests {
		func() {
			waitDep := s.mock.DeploymentReactions(c)
			defer waitDep()
			m := serviceManager{client: s.clusterClient}
			appName := fmt.Sprintf("myapp%d", i)
			a := &appTypes.App{Name: appName, TeamOwner: s.team.Name}
			err := app.CreateApp(context.TODO(), a, s.user)
			c.Assert(err, check.IsNil)
			for j, step := range tt.steps {
				c.Logf("test %v step %v", i, j)
				if step.deployStep != nil {
					versionProcs := map[string]interface{}{}
					procSpec := servicecommon.ProcessSpec{}
					for _, proc := range step.procs {
						versionProcs[proc] = "cmd"
						procSpec[proc] = servicecommon.ProcessState{Start: true}
					}
					version := newCommittedVersion(c, a, map[string]interface{}{
						"processes": versionProcs,
					})
					var oldVersionNumber int
					if !step.deployStep.newVersion {
						oldVersionNumber, err = baseVersionForApp(context.TODO(), s.clusterClient, a)
						c.Assert(err, check.IsNil)
					}
					err = servicecommon.RunServicePipeline(context.TODO(), &m, oldVersionNumber, provision.DeployArgs{
						App:              a,
						Version:          version,
						PreserveVersions: step.deployStep.newVersion,
						OverrideVersions: step.deployStep.overrideVersions,
					}, procSpec)
					c.Assert(err, check.IsNil)
					waitDep()
					a.Deploys++
					if step.deployStep.routable {
						err = app.SetRoutable(context.TODO(), a, version, true)
						c.Assert(err, check.IsNil)
					}
				}
				if step.unitStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.unitStep.version))
					c.Assert(err, check.IsNil)
					err = servicecommon.ChangeUnits(context.TODO(), &m, a, step.unitStep.units, step.unitStep.proc, version)
					c.Assert(err, check.IsNil)
					waitDep()
				}
				if step.stopStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.stopStep.version))
					c.Assert(err, check.IsNil)
					s.updatePastUnits(c, a.Name, version, step.stopStep.proc)
					err = servicecommon.ChangeAppState(context.TODO(), &m, a, step.stopStep.proc, servicecommon.ProcessState{Stop: true}, version)
					c.Assert(err, check.IsNil)
					waitDep()
				}
				if step.startStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
					c.Assert(err, check.IsNil)
					err = servicecommon.ChangeAppState(context.TODO(), &m, a, step.startStep.proc, servicecommon.ProcessState{Start: true}, version)
					c.Assert(err, check.IsNil)
					waitDep()
				}
				if step.restartStep != nil {
					var versions []appTypes.AppVersion
					if step.restartStep.version == 0 {
						versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.restartStep.proc, true)
						c.Assert(err, check.IsNil)
					} else {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
						c.Assert(err, check.IsNil)
						versions = append(versions, version)
					}

					for _, v := range versions {
						err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
							client: s.clusterClient,
							writer: bytes.NewBuffer(nil),
						}, a, step.restartStep.proc, servicecommon.ProcessState{Start: true, Restart: true}, v)
						c.Assert(err, check.IsNil)
						waitDep()
					}
				}
				step.check()
			}
		}()
	}
}
