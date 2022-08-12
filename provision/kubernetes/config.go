package kubernetes

import (
	"context"

	"github.com/tsuru/tsuru/provision"
)

func (p *kubernetesProvisioner) ReloadConfig(ctx context.Context, a provision.App) error {
	client, err := clusterForPool(ctx, a.GetPool())
	if err != nil {
		return err
	}
	return ensureConfigMapForApp(ctx, client, a)
}
