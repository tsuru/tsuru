package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/app"
	check "gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *S) TestReloadConfig(c *check.C) {
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Config: map[string]string{
		"hello.config": "working",
	}}
	err := app.CreateApp(context.TODO(), a, s.user)
	c.Assert(err, check.IsNil)

	err = s.p.ReloadConfig(context.TODO(), a)
	c.Assert(err, check.IsNil)

	configMap, err := s.client.CoreV1().ConfigMaps("default").Get(context.TODO(), "app-myapp", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(configMap.Data, check.DeepEquals, map[string]string{
		"hello.config": "working",
	})
}
