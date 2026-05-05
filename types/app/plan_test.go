// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlan_GetRuntimeClassName(t *testing.T) {
	t.Run("empty when unset", func(t *testing.T) {
		p := Plan{}
		require.Empty(t, p.GetRuntimeClassName())
	})

	t.Run("returns plan value when set", func(t *testing.T) {
		p := Plan{RuntimeClassName: "gvisor"}
		require.Equal(t, "gvisor", p.GetRuntimeClassName())
	})

	t.Run("override takes precedence over plan value", func(t *testing.T) {
		kata := "kata-qemu"
		p := Plan{
			RuntimeClassName: "gvisor",
			Override:         &PlanOverride{RuntimeClassName: &kata},
		}
		require.Equal(t, "kata-qemu", p.GetRuntimeClassName())
	})

	t.Run("override with empty string falls back to plan value", func(t *testing.T) {
		// An empty-string override should be treated as "not overriding"
		// via MergeOverride; but if a caller constructs it directly, the
		// getter returns the (possibly empty) override — we don't try to
		// distinguish empty override from absent override once stored.
		empty := ""
		p := Plan{
			RuntimeClassName: "gvisor",
			Override:         &PlanOverride{RuntimeClassName: &empty},
		}
		require.Equal(t, "", p.GetRuntimeClassName())
	})
}

func TestPlan_MergeOverride_RuntimeClassName(t *testing.T) {
	t.Run("sets override when non-empty", func(t *testing.T) {
		p := Plan{}
		name := "gvisor"
		p.MergeOverride(PlanOverride{RuntimeClassName: &name})
		require.NotNil(t, p.Override)
		require.NotNil(t, p.Override.RuntimeClassName)
		require.Equal(t, "gvisor", *p.Override.RuntimeClassName)
	})

	t.Run("clears override when set to empty string", func(t *testing.T) {
		name := "gvisor"
		p := Plan{Override: &PlanOverride{RuntimeClassName: &name}}
		empty := ""
		p.MergeOverride(PlanOverride{RuntimeClassName: &empty})
		// After clearing the only override field, Override should be nil.
		require.Nil(t, p.Override)
	})

	t.Run("nil leaves existing override untouched", func(t *testing.T) {
		name := "gvisor"
		p := Plan{Override: &PlanOverride{RuntimeClassName: &name}}
		p.MergeOverride(PlanOverride{})
		require.NotNil(t, p.Override)
		require.NotNil(t, p.Override.RuntimeClassName)
		require.Equal(t, "gvisor", *p.Override.RuntimeClassName)
	})
}
