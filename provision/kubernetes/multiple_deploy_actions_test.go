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
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestServiceManagerDeployMultipleFlows(c *check.C) {
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
					deployStep: &deployStep{procs: []string{"p1", "p2"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 1, 1)
						s.hasSvc(c, "myapp0-p1")

						s.noSvc(c, "myapp0-p1-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1", "p2"}, newVersion: true, routable: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp0-p1-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp0-p2-v2", 2, 1)
						s.hasSvc(c, "myapp0-p1")
						s.hasSvc(c, "myapp0-p1-v2")
						s.hasSvc(c, "myapp0-p2-v2")
					},
				},
				{
					unitStep: &unitStep{version: 2, units: 2, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1-v2", 2, 3)

						s.hasDepWithVersion(c, "myapp0-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp0-p2-v2", 2, 1)
						s.hasSvc(c, "myapp0-p1")
						s.hasSvc(c, "myapp0-p1-v2")
						s.hasSvc(c, "myapp0-p2-v2")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1", "p2"}, overrideVersions: true, routable: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp0-p1", 3, 4)
						s.hasDepWithVersion(c, "myapp0-p2", 3, 2)
						s.hasSvc(c, "myapp0-p1")

						s.noDep(c, "myapp0-p1-v2")
						s.noDep(c, "myapp0-p2-v2")
						s.noSvc(c, "myapp0-p1-v2")
						s.noSvc(c, "myapp0-p2-v2")
					},
				},
			},
		},
		{
			steps: []stepDef{
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp1-p1", 1, 1)
						s.hasSvc(c, "myapp1-p1")
						s.noSvc(c, "myapp1-p1-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp1-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp1-p1-v2", 2, 1)
						s.hasSvc(c, "myapp1-p1")

						s.hasSvc(c, "myapp1-p1-v2")
						s.hasSvc(c, "myapp1-p1-v1")
					},
				},
				{
					stopStep: &stopStep{proc: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp1-p1", 1, 0)
						s.hasDepWithVersion(c, "myapp1-p1-v2", 2, 1)
						s.hasSvc(c, "myapp1-p1")
						s.hasSvc(c, "myapp1-p1-v2")
					},
				},
				{
					restartStep: &restartStep{proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp1-p1", 1, 0)
						s.hasDepWithVersion(c, "myapp1-p1-v2", 2, 1)
						s.hasSvc(c, "myapp1-p1")
						s.hasSvc(c, "myapp1-p1-v2")
					},
				},
			},
		},
		{
			steps: []stepDef{
				{
					deployStep: &deployStep{procs: []string{"p1", "p2"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp2-p2", 1, 1)
						s.hasSvc(c, "myapp2-p1")
						s.hasSvc(c, "myapp2-p2")
						s.noSvc(c, "myapp2-p1-v1")
						s.noSvc(c, "myapp2-p2-v1")
					},
				},
				{
					unitStep: &unitStep{proc: "p1", version: 1, units: 2},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p1", 1, 3)
					},
				},
				{
					unitStep: &unitStep{proc: "p2", version: 1, units: 3},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p2", 1, 4)
					},
				},
				{
					stopStep: &stopStep{proc: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p2", 1, 4)
						s.noSvc(c, "myapp2-p1-v1")
					},
				},
				{
					stopStep: &stopStep{proc: "p2", version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p2", 1, 0)
						s.hasDepWithVersion(c, "myapp2-p1", 1, 0)
					},
				},
				{
					startStep: &startStep{proc: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p1", 1, 3)
					},
				},
				{
					startStep: &startStep{proc: "p2", version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp2-p2", 1, 4)
					},
				},
			},
		},
		{
			steps: []stepDef{
				{
					deployStep: &deployStep{procs: []string{"p1", "p2", "p3"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp3-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 1)
						s.hasSvc(c, "myapp3-p1")
						s.hasSvc(c, "myapp3-p2")
						s.hasSvc(c, "myapp3-p3")
						s.noSvc(c, "myapp3-p1-v1")
						s.noSvc(c, "myapp3-p2-v1")
						s.noSvc(c, "myapp3-p3-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1", "p2", "p3"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp3-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p1-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp3-p2-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp3-p2-v2", 2, 1)
						s.hasSvc(c, "myapp3-p1-v1")
						s.hasSvc(c, "myapp3-p2-v1")
						s.hasSvc(c, "myapp3-p3-v1")
						s.hasSvc(c, "myapp3-p1-v2")
						s.hasSvc(c, "myapp3-p2-v2")
						s.hasSvc(c, "myapp3-p3-v2")
					},
				},
				{
					stopStep: &stopStep{},
					check: func() {
						s.hasDepWithVersion(c, "myapp3-p1", 1, 0)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 0)
						s.hasDepWithVersion(c, "myapp3-p3", 1, 0)
						s.noDep(c, "myapp3-p1-v2")
						s.noDep(c, "myapp3-p2-v2")
						s.noDep(c, "myapp3-p3-v2")
					},
				},
				{
					startStep: &startStep{version: 1},
					check: func() {
						s.hasDepWithVersion(c, "myapp3-p1", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p2", 1, 1)
						s.hasDepWithVersion(c, "myapp3-p3", 1, 1)
					},
				},
			},
		},
		{
			steps: []stepDef{
				{
					deployStep: &deployStep{procs: []string{"p1", "p2", "p3"}},
					check:      func() {},
				},
				{
					deployStep: &deployStep{procs: []string{"p1", "p2", "p3"}, newVersion: true},
					check:      func() {},
				},
				{
					unitStep: &unitStep{version: 1, units: 1, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p1", 1, 2)
					},
				},
				{
					unitStep: &unitStep{version: 1, units: 1, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p2", 1, 2)
					},
				}, {
					unitStep: &unitStep{version: 1, units: 1, proc: "p3"},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p3", 1, 2)
					},
				},
				{
					stopStep: &stopStep{},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p1", 1, 0)
						s.hasDepWithVersion(c, "myapp4-p2", 1, 0)
						s.hasDepWithVersion(c, "myapp4-p3", 1, 0)
						s.noDep(c, "myapp4-p1-v2")
						s.noDep(c, "myapp4-p2-v2")
						s.noDep(c, "myapp4-p3-v2")
					},
				},
				{
					startStep: &startStep{},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp4-p3-v2", 2, 1)
					},
				},

				// redundant start
				// previous, we had a bug that doubles the number of units
				// to prevent regression we need to test this case
				{
					startStep: &startStep{},
					check: func() {
						s.hasDepWithVersion(c, "myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion(c, "myapp4-p3-v2", 2, 1)
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
					versions := []appTypes.AppVersion{}
					if step.unitStep.version == 0 {
						versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.unitStep.proc, true)
						c.Assert(err, check.IsNil)
					} else {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.unitStep.version))
						c.Assert(err, check.IsNil)
						versions = append(versions, version)
					}
					for _, v := range versions {
						err = servicecommon.ChangeUnits(context.TODO(), &m, a, step.unitStep.units, step.unitStep.proc, v)
						c.Assert(err, check.IsNil)
						waitDep()
					}
				}
				if step.stopStep != nil {
					versions := []appTypes.AppVersion{}
					if step.stopStep.version != 0 {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.stopStep.version))
						c.Assert(err, check.IsNil)
						s.updatePastUnits(c, a.Name, version, step.stopStep.proc)
						versions = append(versions, version)
					} else {
						versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.stopStep.proc, true)
						c.Assert(err, check.IsNil)
						for _, v := range versions {
							s.updatePastUnitsAllProcesses(c, a.Name, v)
						}
					}
					for _, v := range versions {
						err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
							client: s.clusterClient,
							writer: bytes.NewBuffer(nil),
						}, a, step.stopStep.proc, servicecommon.ProcessState{Stop: true}, v)
						c.Assert(err, check.IsNil)
						waitDep()
					}
				}
				if step.startStep != nil {
					versions := []appTypes.AppVersion{}
					if step.startStep.version == 0 {
						versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.startStep.proc, true)
						c.Assert(err, check.IsNil)
					} else {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
						c.Assert(err, check.IsNil)
						versions = append(versions, version)
					}
					if len(versions) == 0 {
						var version appTypes.AppVersion
						version, err = servicemanager.AppVersion.LatestSuccessfulVersion(context.TODO(), a)
						c.Assert(err, check.IsNil)
						versions = append(versions, version)
					}
					for _, v := range versions {
						err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
							client: s.clusterClient,
							writer: bytes.NewBuffer(nil),
						}, a, step.startStep.proc, servicecommon.ProcessState{Start: true}, v)
						c.Assert(err, check.IsNil)
						waitDep()
					}
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
				waitDep()
				step.check()
			}
		}()
	}
}

func (s *S) hasDepWithVersion(c *check.C, name string, version, replicas int) {
	dep, err := s.client.Clientset.AppsV1().Deployments("default").Get(context.TODO(), name, metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Check(*dep.Spec.Replicas, check.Equals, int32(replicas))
	c.Check(dep.Spec.Template.Labels["tsuru.io/app-version"], check.Equals, strconv.Itoa(version))
}

func (s *S) hasSvc(c *check.C, name string) {
	_, err := s.client.Clientset.CoreV1().Services("default").Get(context.TODO(), name, metav1.GetOptions{})
	c.Check(err, check.IsNil)
}

func (s *S) noSvc(c *check.C, name string) {
	_, err := s.client.Clientset.CoreV1().Services("default").Get(context.TODO(), name, metav1.GetOptions{})
	c.Check(k8sErrors.IsNotFound(err), check.Equals, true)
}

func (s *S) noDep(c *check.C, name string) {
	_, err := s.client.Clientset.AppsV1().Deployments("default").Get(context.TODO(), name, metav1.GetOptions{})
	c.Check(k8sErrors.IsNotFound(err), check.Equals, true)
}

func (s *S) updatePastUnits(c *check.C, appName string, v appTypes.AppVersion, p string) {
	nameLabel := "app=" + appName
	if p != "" {
		nameLabel = "app=" + appName + "-" + p
	}
	deps, err := s.client.Clientset.AppsV1().Deployments("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: nameLabel,
	})
	c.Assert(err, check.IsNil)
	for _, dep := range deps.Items {
		if version, ok := dep.Spec.Template.Labels["tsuru.io/app-version"]; ok {
			if dep.Spec.Replicas != nil && strconv.Itoa(v.Version()) == version {
				err = v.UpdatePastUnits(p, int(*dep.Spec.Replicas))
				c.Assert(err, check.IsNil)
			}
		}
	}
}

func (s *S) updatePastUnitsAllProcesses(c *check.C, appName string, v appTypes.AppVersion) {
	deps, err := s.client.Clientset.AppsV1().Deployments("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tsuru.io/app-name=%s", appName),
	})
	c.Assert(err, check.IsNil)
	for _, dep := range deps.Items {
		if version, ok := dep.Spec.Template.Labels["tsuru.io/app-version"]; ok {
			if strconv.Itoa(v.Version()) == version && dep.Spec.Replicas != nil {
				if process, ok := dep.Spec.Template.Labels["tsuru.io/app-process"]; ok {
					err = v.UpdatePastUnits(process, int(*dep.Spec.Replicas))
					c.Assert(err, check.IsNil)
				}
			}
		}
	}
}
