package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/stretchr/testify/require"
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
						s.hasDepWithVersion("myapp0-p1", 1, 1)
						s.hasSvc("myapp0-p1")

						s.noSvc("myapp0-p1-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 2, 1)
						s.hasSvc("myapp0-p1")

						s.noSvc("myapp0-p1-v2")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 2, 1)
						s.hasDepWithVersion("myapp0-p1-v3", 3, 1)
						s.hasSvc("myapp0-p1")
						s.hasSvc("myapp0-p1-v2")
						s.hasSvc("myapp0-p1-v3")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 4, 1)
						s.hasSvc("myapp0-p1")

						s.noDep("myapp0-p1-v3")
						s.noSvc("myapp0-p1-v2")
						s.noSvc("myapp0-p1-v3")
						s.noSvc("myapp0-p1-v4")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p2"}},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 1)
						s.hasSvc("myapp0-p2")

						s.noDep("myapp0-p1")
						s.noSvc("myapp0-p1")
						s.noSvc("myapp0-p2-v5")
					},
				},
				{
					unitStep: &unitStep{version: 4, units: 2, proc: "p1"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 1)
						s.hasSvc("myapp0-p2")
						s.hasDepWithVersion("myapp0-p1-v4", 4, 2)
						s.hasSvc("myapp0-p1-v4")

						s.noDep("myapp0-p1")
						s.noSvc("myapp0-p2-v5")
						s.noSvc("myapp0-p1")
					},
				},
				{
					stopStep: &stopStep{version: 4, proc: "p1"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 1)
						s.hasSvc("myapp0-p2")

						s.hasDepWithVersion("myapp0-p1-v4", 4, 0)
						s.noDep("myapp0-p1")
						s.hasSvc("myapp0-p1-v4")
						s.noSvc("myapp0-p1")
						s.noSvc("myapp0-p2-v5")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 0)
						s.hasSvc("myapp0-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 1)
						s.hasSvc("myapp0-p2")
					},
				},
				{
					unitStep: &unitStep{version: 5, units: 2, proc: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 3)
						s.hasSvc("myapp0-p2")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 0)
						s.hasSvc("myapp0-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp0-p2", 5, 3)
						s.hasSvc("myapp0-p2")
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
			require.NoError(s.t, err)
			for j, step := range tt.steps {
				c.Logf("test %v step %v", i, j)
				if step.deployStep != nil {
					versionProcs := map[string][]string{}
					procSpec := servicecommon.ProcessSpec{}
					for _, proc := range step.procs {
						versionProcs[proc] = []string{"cmd"}
						procSpec[proc] = servicecommon.ProcessState{Start: true}
					}
					version := newCommittedVersion(c, a, versionProcs)
					var oldVersionNumber int
					if !step.deployStep.newVersion {
						oldVersionNumber, err = baseVersionForApp(context.TODO(), s.clusterClient, a)
						require.NoError(s.t, err)
					}
					err = servicecommon.RunServicePipeline(context.TODO(), &m, oldVersionNumber, provision.DeployArgs{
						App:              a,
						Version:          version,
						PreserveVersions: step.deployStep.newVersion,
						OverrideVersions: step.deployStep.overrideVersions,
					}, procSpec)
					require.NoError(s.t, err)
					waitDep()
					a.Deploys++
					if step.deployStep.routable {
						err = app.SetRoutable(context.TODO(), a, version, true)
						require.NoError(s.t, err)
					}
				}
				if step.unitStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.unitStep.version))
					require.NoError(s.t, err)
					err = servicecommon.ChangeUnits(context.TODO(), &m, a, step.unitStep.units, step.unitStep.proc, version)
					require.NoError(s.t, err)
					waitDep()
				}
				if step.stopStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.stopStep.version))
					require.NoError(s.t, err)
					s.updatePastUnits(a.Name, version, step.stopStep.proc)
					err = servicecommon.ChangeAppState(context.TODO(), &m, a, step.stopStep.proc, servicecommon.ProcessState{Stop: true}, version)
					require.NoError(s.t, err)
					waitDep()
				}
				if step.startStep != nil {
					var version appTypes.AppVersion
					version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
					require.NoError(s.t, err)
					err = servicecommon.ChangeAppState(context.TODO(), &m, a, step.startStep.proc, servicecommon.ProcessState{Start: true}, version)
					require.NoError(s.t, err)
					waitDep()
				}
				if step.restartStep != nil {
					var versions []appTypes.AppVersion
					if step.restartStep.version == 0 {
						versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.restartStep.proc, true)
						require.NoError(s.t, err)
					} else {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
						require.NoError(s.t, err)
						versions = append(versions, version)
					}

					for _, v := range versions {
						err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
							client: s.clusterClient,
							writer: bytes.NewBuffer(nil),
						}, a, step.restartStep.proc, servicecommon.ProcessState{Start: true, Restart: true}, v)
						require.NoError(s.t, err)
						waitDep()
					}
				}
				step.check()
			}
		}()
	}
}
