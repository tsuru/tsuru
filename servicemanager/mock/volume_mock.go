package mock

import (
	"context"

	volumeTypes "github.com/tsuru/tsuru/types/volume"
)

type MockVolumeService struct {
	Storage volumeTypes.VolumeStorage

	OnVolumeService              func() (volumeTypes.Volume, error)
	OnVolumeStorage              func() (volumeTypes.VolumeStorage, error)
	OnCreate                     func(ctx context.Context, v *volumeTypes.Volume) error
	OnUpdate                     func(ctx context.Context, v *volumeTypes.Volume) error
	OnGet                        func(ctx context.Context, appName string) (*volumeTypes.Volume, error)
	OnListByApp                  func(ctx context.Context, appName string) ([]volumeTypes.Volume, error)
	OnListByFilter               func(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error)
	OnDelete                     func(ctx context.Context, v *volumeTypes.Volume) error
	OnBindApp                    func(ctx context.Context, opts *volumeTypes.BindOpts) error
	OnUnbindApp                  func(ctx context.Context, opts *volumeTypes.BindOpts) error
	OnBinds                      func(ctx context.Context, v *volumeTypes.Volume) ([]volumeTypes.VolumeBind, error)
	OnBindsForApp                func(ctx context.Context, v *volumeTypes.Volume, appName string) ([]volumeTypes.VolumeBind, error)
	OnListPlans                  func(ctx context.Context) (map[string][]volumeTypes.VolumePlan, error)
	OnCheckPoolVolumeConstraints func(ctx context.Context, volume volumeTypes.Volume) error
}

func (m *MockVolumeService) VolumeService() (volumeTypes.Volume, error) {
	if m.OnVolumeService != nil {
		return m.OnVolumeService()
	}
	return volumeTypes.Volume{}, nil
}

func (m *MockVolumeService) VolumeStorage() (volumeTypes.VolumeStorage, error) {
	if m.OnVolumeStorage != nil {
		return m.OnVolumeStorage()
	}
	return nil, nil
}

func (m *MockVolumeService) Create(ctx context.Context, v *volumeTypes.Volume) error {
	if m.OnCreate != nil {
		return m.OnCreate(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) Update(ctx context.Context, v *volumeTypes.Volume) error {
	if m.OnUpdate != nil {
		return m.OnUpdate(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) Get(ctx context.Context, appName string) (*volumeTypes.Volume, error) {
	if m.OnGet != nil {
		return m.OnGet(ctx, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListByApp(ctx context.Context, appName string) ([]volumeTypes.Volume, error) {
	if m.OnListByApp != nil {
		return m.OnListByApp(ctx, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListByFilter(ctx context.Context, f *volumeTypes.Filter) ([]volumeTypes.Volume, error) {
	if m.OnListByFilter != nil {
		return m.OnListByFilter(ctx, f)
	}
	return nil, nil
}

func (m *MockVolumeService) Delete(ctx context.Context, v *volumeTypes.Volume) error {
	if m.OnDelete != nil {
		return m.OnDelete(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) BindApp(ctx context.Context, opts *volumeTypes.BindOpts) error {
	if m.OnBindApp != nil {
		return m.OnBindApp(ctx, opts)
	}
	return nil
}

func (m *MockVolumeService) UnbindApp(ctx context.Context, opts *volumeTypes.BindOpts) error {
	if m.OnUnbindApp != nil {
		return m.OnUnbindApp(ctx, opts)
	}
	return nil
}

func (m *MockVolumeService) Binds(ctx context.Context, v *volumeTypes.Volume) ([]volumeTypes.VolumeBind, error) {
	if m.OnBinds != nil {
		return m.OnBinds(ctx, v)
	}
	return nil, nil
}

func (m *MockVolumeService) BindsForApp(ctx context.Context, v *volumeTypes.Volume, appName string) ([]volumeTypes.VolumeBind, error) {
	if m.OnBindsForApp != nil {
		return m.OnBindsForApp(ctx, v, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListPlans(ctx context.Context) (map[string][]volumeTypes.VolumePlan, error) {
	if m.OnListPlans != nil {
		return m.OnListPlans(ctx)
	}
	return nil, nil
}

func (m *MockVolumeService) CheckPoolVolumeConstraints(ctx context.Context, volume volumeTypes.Volume) error {
	if m.OnCheckPoolVolumeConstraints != nil {
		return m.OnCheckPoolVolumeConstraints(ctx, volume)
	}
	return nil
}
