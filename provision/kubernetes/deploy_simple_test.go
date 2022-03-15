package kubernetes

import (
	"context"
	"strconv"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	check "gopkg.in/check.v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestServiceManagerDeployMulti(c *check.C) {
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
						s.hasDepWithVersion(c, "myapp-p1", 1, 1)
						s.hasSvc(c, "myapp-p1")

						s.noSvc(c, "myapp-p1-v1")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p1", 2, 1)
						s.hasSvc(c, "myapp-p1")

						s.noSvc(c, "myapp-p1-v2")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}, newVersion: true, routable: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p1", 2, 1)
						s.hasDepWithVersion(c, "myapp-p1-v3", 3, 1)
						s.hasSvc(c, "myapp-p1")
						s.hasSvc(c, "myapp-p1-v2")
						s.hasSvc(c, "myapp-p1-v3")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p1"}, overrideVersions: true},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p1", 4, 2)
						s.hasSvc(c, "myapp-p1")

						s.noDep(c, "myapp-p1-v3")
						s.noSvc(c, "myapp-p1-v2")
						s.noSvc(c, "myapp-p1-v3")
						s.noSvc(c, "myapp-p1-v4")
					},
				},
				{
					deployStep: &deployStep{procs: []string{"p2"}},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 1)
						s.hasSvc(c, "myapp-p2")

						s.noDep(c, "myapp-p1")
						s.noSvc(c, "myapp-p1")
						s.noSvc(c, "myapp-p2-v5")
					},
				},
				{
					unitStep: &unitStep{version: 4, units: 2, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 1)
						s.hasSvc(c, "myapp-p2")
						s.hasDepWithVersion(c, "myapp-p1-v4", 4, 2)
						s.hasSvc(c, "myapp-p1-v4")

						s.noDep(c, "myapp-p1")
						s.noSvc(c, "myapp-p2-v5")
						s.noSvc(c, "myapp-p1")
					},
				},
				{
					stopStep: &stopStep{version: 4, proc: "p1"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 1)
						s.hasSvc(c, "myapp-p2")

						s.noDep(c, "myapp-p1-v4")
						s.noDep(c, "myapp-p1")
						s.noSvc(c, "myapp-p1-v4")
						s.noSvc(c, "myapp-p1")
						s.noSvc(c, "myapp-p2-v5")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 0)
						s.hasSvc(c, "myapp-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 1)
						s.hasSvc(c, "myapp-p2")
					},
				},
				{
					unitStep: &unitStep{version: 5, units: 2, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 3)
						s.hasSvc(c, "myapp-p2")
					},
				},
				{
					stopStep: &stopStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 0)
						s.hasSvc(c, "myapp-p2")
					},
				},
				{
					startStep: &startStep{version: 5, proc: "p2"},
					check: func() {
						s.hasDepWithVersion(c, "myapp-p2", 5, 3)
						s.hasSvc(c, "myapp-p2")
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
			a := &app.App{Name: "myapp", TeamOwner: s.team.Name}
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
					if step.deployStep.routable {
						err = a.SetRoutable(context.TODO(), version, true)
						c.Assert(err, check.IsNil)
					}
				}
				if step.unitStep != nil {
					version, err := servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.unitStep.version))
					c.Assert(err, check.IsNil)
					err = servicecommon.RunServicePipeline(context.TODO(), &m, version.Version(), provision.DeployArgs{
						App:              a,
						Version:          version,
						PreserveVersions: true,
					}, servicecommon.ProcessSpec{
						step.unitStep.proc: servicecommon.ProcessState{Increment: step.unitStep.units, Start: true},
					})
					c.Assert(err, check.IsNil)
					waitDep()
				}
				if step.stopStep != nil {
					version, err := servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.stopStep.version))
					c.Assert(err, check.IsNil)
					err = servicecommon.RunServicePipeline(context.TODO(), &m, version.Version(), provision.DeployArgs{
						App:              a,
						Version:          version,
						PreserveVersions: true,
					}, servicecommon.ProcessSpec{
						step.stopStep.proc: servicecommon.ProcessState{Stop: true},
					})
					c.Assert(err, check.IsNil)
					waitDep()
				}
				if step.startStep != nil {
					version, err := servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.startStep.version))
					c.Assert(err, check.IsNil)
					err = servicecommon.RunServicePipeline(context.TODO(), &m, version.Version(), provision.DeployArgs{
						App:              a,
						Version:          version,
						PreserveVersions: true,
					}, servicecommon.ProcessSpec{
						step.startStep.proc: servicecommon.ProcessState{Start: true},
					})
					c.Assert(err, check.IsNil)
					waitDep()
				}
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
