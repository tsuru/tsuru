/*
Copyright The CBI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generic

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crd "github.com/containerbuilding/cbi/pkg/apis/cbi/v1alpha1"
	api "github.com/containerbuilding/cbi/pkg/plugin/api"
)

func TestSelectPlugin(t *testing.T) {
	plugins := []api.InfoResponse{
		{
			// 0
			Labels: map[string]string{
				api.LPluginName:         "foo",
				api.LLanguageDockerfile: "",
			},
		},
		{
			// 1
			Labels: map[string]string{
				api.LPluginName:         "foo",
				api.LLanguageDockerfile: "",
				api.LContextGit:         "",
			},
		},
		{
			// 2
			Labels: map[string]string{
				api.LPluginName:         "bar",
				api.LLanguageDockerfile: "",
				api.LContextGit:         "",
			},
		},
	}

	testCases := []struct {
		bj          crd.BuildJob
		expected    int
		expectedErr bool
	}{
		{
			bj: crd.BuildJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy0",
				},
				Spec: crd.BuildJobSpec{
					Language: crd.Language{
						Kind: crd.LanguageKindDockerfile,
					},
					Context: crd.Context{
						Kind: crd.ContextKindGit,
					},
				},
			},
			expected: 1,
		},
		{
			bj: crd.BuildJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy1",
				},
				Spec: crd.BuildJobSpec{
					Language: crd.Language{
						Kind: crd.LanguageKindDockerfile,
					},
					Context: crd.Context{
						Kind: crd.ContextKindGit,
					},
					PluginSelector: "plugin.name == bar",
				},
			},
			expected: 2,
		},
		{
			bj: crd.BuildJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy2",
				},
				Spec: crd.BuildJobSpec{
					Language: crd.Language{
						Kind: crd.LanguageKindDockerfile,
					},
					Context: crd.Context{
						Kind: crd.ContextKindGit,
					},
					PluginSelector: "plugin.name == nx",
				},
			},
			expectedErr: true,
		},
	}
	for _, tc := range testCases {
		actual, err := SelectPlugin(plugins, tc.bj)
		if err != nil && !tc.expectedErr {
			t.Fatalf("%s: %v", tc.bj.Name, err)
		}
		if err == nil {
			if tc.expectedErr {
				t.Fatalf("%s: error is expected", tc.bj.Name)
			} else if tc.expected != actual {
				t.Fatalf("%s: expected %d, got %d", tc.bj.Name, tc.expected, actual)
			}
		}
	}
}
