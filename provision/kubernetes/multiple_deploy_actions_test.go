package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/stretchr/testify/require"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/servicecommon"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	check "gopkg.in/check.v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type deployStep struct {
	processes           []string
	newVersion          bool
	overrideOldVersions bool
	routable            bool
}

type unitStep struct {
	process string
	version int
	units   int
}

type stopStep struct {
	process string
	version int
}

type startStep struct {
	proc    string
	version int
}

type restartStep struct {
	proc    string
	version int
}

type stepDef struct {
	*restartStep
	*stopStep
	*startStep
	*unitStep
	*deployStep
	check func()
}

func (s *S) TestServiceManagerDeployMultipleFlows(c *check.C) {
	tests := []struct {
		testTitle string
		steps     []stepDef
	}{
		{
			testTitle: "Overriding old versions with multiple processes should conserve the number of units",
			steps: []stepDef{
				{
					deployStep: &deployStep{processes: []string{"p1", "p2"}},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 1, 1)
						s.hasSvc("myapp0-p1")

						s.noSvc("myapp0-p1-v1")
					},
				},
				{
					deployStep: &deployStep{processes: []string{"p1", "p2"}, newVersion: true, routable: true},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 1, 1)
						s.hasDepWithVersion("myapp0-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp0-p2-v2", 2, 1)
						s.hasSvc("myapp0-p1")
						s.hasSvc("myapp0-p1-v2")
						s.hasSvc("myapp0-p2-v2")
					},
				},
				{
					unitStep: &unitStep{version: 2, units: 2, process: "p1"},
					check: func() {
						s.hasDepWithVersion("myapp0-p1-v2", 2, 3)

						s.hasDepWithVersion("myapp0-p1", 1, 1)
						s.hasDepWithVersion("myapp0-p2-v2", 2, 1)
						s.hasSvc("myapp0-p1")
						s.hasSvc("myapp0-p1-v2")
						s.hasSvc("myapp0-p2-v2")
					},
				},
				{
					deployStep: &deployStep{processes: []string{"p1", "p2"}, overrideOldVersions: true, routable: true},
					check: func() {
						s.hasDepWithVersion("myapp0-p1", 3, 4)
						s.hasDepWithVersion("myapp0-p2", 3, 2)
						s.hasSvc("myapp0-p1")

						s.noDep("myapp0-p1-v2")
						s.noDep("myapp0-p2-v2")
						s.noSvc("myapp0-p1-v2")
						s.noSvc("myapp0-p2-v2")
					},
				},
			},
		},
		{
			testTitle: "Restarting stopped process versions should maintain the number of units as 0",
			steps: []stepDef{
				{
					deployStep: &deployStep{processes: []string{"p1"}},
					check: func() {
						s.hasDepWithVersion("myapp1-p1", 1, 1)
						s.hasSvc("myapp1-p1")
						s.noSvc("myapp1-p1-v1")
					},
				},
				{
					deployStep: &deployStep{processes: []string{"p1"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion("myapp1-p1", 1, 1)
						s.hasDepWithVersion("myapp1-p1-v2", 2, 1)
						s.hasSvc("myapp1-p1")

						s.hasSvc("myapp1-p1-v2")
						s.hasSvc("myapp1-p1-v1")
					},
				},
				{
					stopStep: &stopStep{process: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion("myapp1-p1", 1, 0)
						s.hasDepWithVersion("myapp1-p1-v2", 2, 1)
						s.hasSvc("myapp1-p1")
						s.hasSvc("myapp1-p1-v2")
					},
				},
				{
					restartStep: &restartStep{proc: "p1"},
					check: func() {
						s.hasDepWithVersion("myapp1-p1", 1, 0)
						s.hasDepWithVersion("myapp1-p1-v2", 2, 1)
						s.hasSvc("myapp1-p1")
						s.hasSvc("myapp1-p1-v2")
					},
				},
			},
		},
		{
			testTitle: "Starting stopped process versions should maintain the previous number of units",
			steps: []stepDef{
				{
					deployStep: &deployStep{processes: []string{"p1", "p2"}},
					check: func() {
						s.hasDepWithVersion("myapp2-p1", 1, 1)
						s.hasDepWithVersion("myapp2-p2", 1, 1)
						s.hasSvc("myapp2-p1")
						s.hasSvc("myapp2-p2")
						s.noSvc("myapp2-p1-v1")
						s.noSvc("myapp2-p2-v1")
					},
				},
				{
					unitStep: &unitStep{process: "p1", version: 1, units: 2},
					check: func() {
						s.hasDepWithVersion("myapp2-p1", 1, 3)
					},
				},
				{
					unitStep: &unitStep{process: "p2", version: 1, units: 3},
					check: func() {
						s.hasDepWithVersion("myapp2-p2", 1, 4)
					},
				},
				{
					stopStep: &stopStep{process: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion("myapp2-p2", 1, 4)
						s.noSvc("myapp2-p1-v1")
					},
				},
				{
					stopStep: &stopStep{process: "p2", version: 1},
					check: func() {
						s.hasDepWithVersion("myapp2-p2", 1, 0)
						s.hasDepWithVersion("myapp2-p1", 1, 0)
					},
				},
				{
					startStep: &startStep{proc: "p1", version: 1},
					check: func() {
						s.hasDepWithVersion("myapp2-p1", 1, 3)
					},
				},
				{
					startStep: &startStep{proc: "p2", version: 1},
					check: func() {
						s.hasDepWithVersion("myapp2-p2", 1, 4)
					},
				},
			},
		},
		{
			testTitle: "Starting all process by version should maintain other versions stopped, when stopped should maintain deployments",
			steps: []stepDef{
				{
					deployStep: &deployStep{processes: []string{"p1", "p2", "p3"}},
					check: func() {
						s.hasDepWithVersion("myapp3-p1", 1, 1)
						s.hasDepWithVersion("myapp3-p2", 1, 1)
						s.hasDepWithVersion("myapp3-p2", 1, 1)
						s.hasSvc("myapp3-p1")
						s.hasSvc("myapp3-p2")
						s.hasSvc("myapp3-p3")
						s.noSvc("myapp3-p1-v1")
						s.noSvc("myapp3-p2-v1")
						s.noSvc("myapp3-p3-v1")
					},
				},
				{
					deployStep: &deployStep{processes: []string{"p1", "p2", "p3"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion("myapp3-p1", 1, 1)
						s.hasDepWithVersion("myapp3-p2", 1, 1)
						s.hasDepWithVersion("myapp3-p2", 1, 1)
						s.hasDepWithVersion("myapp3-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp3-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp3-p2-v2", 2, 1)
						s.hasSvc("myapp3-p1-v1")
						s.hasSvc("myapp3-p2-v1")
						s.hasSvc("myapp3-p3-v1")
						s.hasSvc("myapp3-p1-v2")
						s.hasSvc("myapp3-p2-v2")
						s.hasSvc("myapp3-p3-v2")
					},
				},
				{
					stopStep: &stopStep{},
					check: func() {
						s.hasDepWithVersion("myapp3-p1", 1, 0)
						s.hasDepWithVersion("myapp3-p2", 1, 0)
						s.hasDepWithVersion("myapp3-p3", 1, 0)
						s.hasDepWithVersion("myapp3-p1-v2", 2, 0)
						s.hasDepWithVersion("myapp3-p2-v2", 2, 0)
						s.hasDepWithVersion("myapp3-p3-v2", 2, 0)
					},
				},
				{
					startStep: &startStep{version: 1},
					check: func() {
						s.hasDepWithVersion("myapp3-p1", 1, 1)
						s.hasDepWithVersion("myapp3-p2", 1, 1)
						s.hasDepWithVersion("myapp3-p3", 1, 1)
						s.hasDepWithVersion("myapp3-p1-v2", 2, 0)
						s.hasDepWithVersion("myapp3-p2-v2", 2, 0)
						s.hasDepWithVersion("myapp3-p3-v2", 2, 0)
					},
				},
			},
		},
		{
			testTitle: "Starting all processes should only start all versions - start running app should not change number of units",
			steps: []stepDef{
				{
					deployStep: &deployStep{processes: []string{"p1", "p2", "p3"}},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 1)
						s.hasDepWithVersion("myapp4-p2", 1, 1)
						s.hasDepWithVersion("myapp4-p3", 1, 1)
					},
				},
				{
					deployStep: &deployStep{processes: []string{"p1", "p2", "p3"}, newVersion: true},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 1)
						s.hasDepWithVersion("myapp4-p2", 1, 1)
						s.hasDepWithVersion("myapp4-p3", 1, 1)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
					},
				},
				{
					unitStep: &unitStep{version: 1, units: 1, process: "p1"},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 2)
						s.hasDepWithVersion("myapp4-p2", 1, 1)
						s.hasDepWithVersion("myapp4-p3", 1, 1)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
					},
				},
				{
					unitStep: &unitStep{version: 1, units: 1, process: "p2"},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 2)
						s.hasDepWithVersion("myapp4-p2", 1, 2)
						s.hasDepWithVersion("myapp4-p3", 1, 1)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
					},
				},
				{
					unitStep: &unitStep{version: 1, units: 1, process: "p3"},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 2)
						s.hasDepWithVersion("myapp4-p2", 1, 2)
						s.hasDepWithVersion("myapp4-p3", 1, 2)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
					},
				},
				{
					stopStep: &stopStep{},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 0)
						s.hasDepWithVersion("myapp4-p2", 1, 0)
						s.hasDepWithVersion("myapp4-p3", 1, 0)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 0)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 0)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 0)
						s.hasSvc("myapp4-p1")
						s.hasSvc("myapp4-p1-v1")
						s.hasSvc("myapp4-p2")
						s.hasSvc("myapp4-p2-v1")
						s.hasSvc("myapp4-p3")
						s.hasSvc("myapp4-p3-v1")
						s.hasSvc("myapp4-p1-v2")
						s.hasSvc("myapp4-p2-v2")
						s.hasSvc("myapp4-p3-v2")
					},
				},
				{
					startStep: &startStep{},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 2)
						s.hasDepWithVersion("myapp4-p2", 1, 2)
						s.hasDepWithVersion("myapp4-p3", 1, 2)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
						s.hasSvc("myapp4-p1")
						s.hasSvc("myapp4-p1-v1")
						s.hasSvc("myapp4-p2")
						s.hasSvc("myapp4-p2-v1")
						s.hasSvc("myapp4-p3")
						s.hasSvc("myapp4-p3-v1")
						s.hasSvc("myapp4-p1-v2")
						s.hasSvc("myapp4-p2-v2")
						s.hasSvc("myapp4-p3-v2")
					},
				},

				// redundant start
				// previously, we had a bug that doubles the number of units
				// to prevent regression we need to test this case
				{
					startStep: &startStep{},
					check: func() {
						s.hasDepWithVersion("myapp4-p1", 1, 2)
						s.hasDepWithVersion("myapp4-p2", 1, 2)
						s.hasDepWithVersion("myapp4-p3", 1, 2)
						s.hasDepWithVersion("myapp4-p1-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p2-v2", 2, 1)
						s.hasDepWithVersion("myapp4-p3-v2", 2, 1)
						s.hasSvc("myapp4-p1")
						s.hasSvc("myapp4-p1-v1")
						s.hasSvc("myapp4-p2")
						s.hasSvc("myapp4-p2-v1")
						s.hasSvc("myapp4-p3")
						s.hasSvc("myapp4-p3-v1")
						s.hasSvc("myapp4-p1-v2")
						s.hasSvc("myapp4-p2-v2")
						s.hasSvc("myapp4-p3-v2")
					},
				},
			},
		},
	}

	for i, tt := range tests {
		fmt.Printf("running test %d: %s\n", i, tt.testTitle)
		waitDep := s.mock.DeploymentReactions(c)
		defer waitDep()
		m := serviceManager{client: s.clusterClient}
		appName := fmt.Sprintf("myapp%d", i)
		a := &appTypes.App{Name: appName, TeamOwner: s.team.Name}
		err := app.CreateApp(context.TODO(), a, s.user)
		require.NoError(s.t, err)
		for j, step := range tt.steps {
			fmt.Printf("step %v\n", j)
			if step.deployStep != nil {
				err := s.deployStep(c, a, &m, step.deployStep, waitDep)
				require.NoError(s.t, err)
			}
			if step.unitStep != nil {
				err := s.unitStep(c, a, &m, step.unitStep, waitDep)
				require.NoError(s.t, err)
			}
			if step.stopStep != nil {
				err := s.stopStep(c, a, &m, step.stopStep, waitDep)
				require.NoError(s.t, err)
			}
			if step.startStep != nil {
				err := s.startStep(c, a, &m, step.startStep, waitDep)
				require.NoError(s.t, err)
			}
			if step.restartStep != nil {
				err := s.restartStep(c, a, &m, step.restartStep, waitDep)
				require.NoError(s.t, err)
			}
			waitDep()
			step.check()
		}
	}
}

func (s *S) deployStep(c *check.C, a *appTypes.App, m *serviceManager, step *deployStep, waitDep func()) error {
	var err error
	var oldVersionNumber int
	versionProcesses := map[string][]string{}
	processSpec := servicecommon.ProcessSpec{}
	for _, proc := range step.processes {
		versionProcesses[proc] = []string{"cmd"}
		processSpec[proc] = servicecommon.ProcessState{Start: true}
	}
	version := newCommittedVersion(c, a, versionProcesses)
	if !step.newVersion {
		oldVersionNumber, err = baseVersionForApp(context.TODO(), s.clusterClient, a)
		if err != nil {
			return err
		}
	}
	err = servicecommon.RunServicePipeline(context.TODO(), m, oldVersionNumber, provision.DeployArgs{
		App:              a,
		Version:          version,
		PreserveVersions: step.newVersion,
		OverrideVersions: step.overrideOldVersions,
	}, processSpec)
	if err != nil {
		return err
	}
	waitDep()
	a.Deploys++
	if step.routable {
		err = app.SetRoutable(context.TODO(), a, version, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *S) unitStep(c *check.C, a *appTypes.App, m *serviceManager, step *unitStep, waitDep func()) error {
	var err error
	versions := []appTypes.AppVersion{}
	if step.version == 0 {
		versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.process, true)
		if err != nil {
			return err
		}
	} else {
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.version))
		if err != nil {
			return err
		}
		versions = append(versions, version)
	}
	for _, v := range versions {
		err = servicecommon.ChangeUnits(context.TODO(), m, a, step.units, step.process, v)
		if err != nil {
			return err
		}
		waitDep()
	}
	return nil
}

func (s *S) stopStep(c *check.C, a *appTypes.App, m *serviceManager, step *stopStep, waitDep func()) error {
	var err error
	versions := []appTypes.AppVersion{}
	if step.version != 0 {
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.version))
		if err != nil {
			return err
		}
		s.updatePastUnits(a.Name, version, step.process)
		versions = append(versions, version)
	} else {
		versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.process, false)
		if err != nil {
			return err
		}
		for _, v := range versions {
			s.updatePastUnitsAllProcesses(a.Name, v)
		}
	}
	slices.SortFunc(versions, func(a, b appTypes.AppVersion) int {
		if a.Version() > b.Version() {
			return -1
		}
		if a.Version() < b.Version() {
			return 1
		}
		return 0
	})
	for _, v := range versions {
		err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
			client: s.clusterClient,
			writer: bytes.NewBuffer(nil),
		}, a, step.process, servicecommon.ProcessState{Stop: true}, v)
		if err != nil {
			return err
		}
		waitDep()
	}
	return nil
}

func (s *S) startStep(c *check.C, a *appTypes.App, m *serviceManager, step *startStep, waitDep func()) error {
	var err error
	versions := []appTypes.AppVersion{}
	if step.version == 0 {
		versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.proc, false)
		if err != nil {
			return err
		}
	} else {
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.version))
		if err != nil {
			return err
		}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.LatestSuccessfulVersion(context.TODO(), a)
		if err != nil {
			return err
		}
		versions = append(versions, version)
	}
	slices.SortFunc(versions, func(a, b appTypes.AppVersion) int {
		if a.Version() > b.Version() {
			return -1
		}
		if a.Version() < b.Version() {
			return 1
		}
		return 0
	})

	for _, v := range versions {
		err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
			client: s.clusterClient,
			writer: bytes.NewBuffer(nil),
		}, a, step.proc, servicecommon.ProcessState{Start: true}, v)
		if err != nil {
			return err
		}
		waitDep()
	}
	return nil
}

func (s *S) restartStep(c *check.C, a *appTypes.App, m *serviceManager, step *restartStep, waitDep func()) error {
	var err error
	var versions []appTypes.AppVersion
	if step.version == 0 {
		versions, err = versionsForAppProcess(context.TODO(), s.clusterClient, a, step.proc, true)
		if err != nil {
			return err
		}
	} else {
		var version appTypes.AppVersion
		version, err = servicemanager.AppVersion.VersionByImageOrVersion(context.TODO(), a, strconv.Itoa(step.version))
		if err != nil {
			return err
		}
		versions = append(versions, version)
	}
	for _, v := range versions {
		err = servicecommon.ChangeAppState(context.TODO(), &serviceManager{
			client: s.clusterClient,
			writer: bytes.NewBuffer(nil),
		}, a, step.proc, servicecommon.ProcessState{Start: true, Restart: true}, v)
		if err != nil {
			return err
		}
		waitDep()
	}
	return nil
}

func (s *S) hasDepWithVersion(name string, version, replicas int) {
	dep, err := s.client.Clientset.AppsV1().Deployments("default").Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(s.t, err)
	require.Equal(s.t, int32(replicas), *dep.Spec.Replicas)
	require.Equal(s.t, strconv.Itoa(version), dep.Spec.Template.Labels["tsuru.io/app-version"])
}

func (s *S) hasSvc(name string) {
	_, err := s.client.Clientset.CoreV1().Services("default").Get(context.TODO(), name, metav1.GetOptions{})
	require.NoError(s.t, err)
}

func (s *S) noSvc(name string) {
	_, err := s.client.Clientset.CoreV1().Services("default").Get(context.TODO(), name, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) noDep(name string) {
	_, err := s.client.Clientset.AppsV1().Deployments("default").Get(context.TODO(), name, metav1.GetOptions{})
	require.True(s.t, k8sErrors.IsNotFound(err))
}

func (s *S) updatePastUnits(appName string, v appTypes.AppVersion, p string) {
	nameLabel := "app=" + appName
	if p != "" {
		nameLabel = "app=" + appName + "-" + p
	}
	deps, err := s.client.Clientset.AppsV1().Deployments("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: nameLabel,
	})
	require.NoError(s.t, err)
	for _, dep := range deps.Items {
		if version, ok := dep.Spec.Template.Labels["tsuru.io/app-version"]; ok {
			if dep.Spec.Replicas != nil && strconv.Itoa(v.Version()) == version {
				err = v.UpdatePastUnits(p, int(*dep.Spec.Replicas))
				require.NoError(s.t, err)
			}
		}
	}
}

func (s *S) updatePastUnitsAllProcesses(appName string, v appTypes.AppVersion) {
	deps, err := s.client.Clientset.AppsV1().Deployments("default").List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tsuru.io/app-name=%s", appName),
	})
	require.NoError(s.t, err)
	for _, dep := range deps.Items {
		if version, ok := dep.Spec.Template.Labels["tsuru.io/app-version"]; ok {
			if strconv.Itoa(v.Version()) == version && dep.Spec.Replicas != nil {
				if process, ok := dep.Spec.Template.Labels["tsuru.io/app-process"]; ok {
					err = v.UpdatePastUnits(process, int(*dep.Spec.Replicas))
					require.NoError(s.t, err)
				}
			}
		}
	}
}
