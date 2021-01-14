package volume

import (
	"context"
)

type MockVolumeService struct {
	Storage MockVolumeStorage

	OnVolumeService              func() (Volume, error)
	OnVolumeStorage              func() (VolumeStorage, error)
	OnCreate                     func(ctx context.Context, v *Volume) error
	OnUpdate                     func(ctx context.Context, v *Volume) error
	OnGet                        func(ctx context.Context, appName string) (*Volume, error)
	OnListByApp                  func(ctx context.Context, appName string) ([]Volume, error)
	OnListByFilter               func(ctx context.Context, f *Filter) ([]Volume, error)
	OnDelete                     func(ctx context.Context, v *Volume) error
	OnBindApp                    func(ctx context.Context, opts *BindOpts) error
	OnUnbindApp                  func(ctx context.Context, opts *BindOpts) error
	OnBinds                      func(ctx context.Context, v *Volume) ([]VolumeBind, error)
	OnBindsForApp                func(ctx context.Context, v *Volume, appName string) ([]VolumeBind, error)
	OnListPlans                  func(ctx context.Context) (map[string][]VolumePlan, error)
	OnCheckPoolVolumeConstraints func(ctx context.Context, volume Volume) error
}

func (m *MockVolumeService) VolumeService() (Volume, error) {
	if m.OnVolumeService != nil {
		return m.OnVolumeService()
	}
	return Volume{}, nil
}

func (m *MockVolumeService) VolumeStorage() (VolumeStorage, error) {
	if m.OnVolumeStorage != nil {
		return m.OnVolumeStorage()
	}
	return nil, nil
}

func (m *MockVolumeService) Create(ctx context.Context, v *Volume) error {
	if m.OnCreate != nil {
		return m.OnCreate(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) Update(ctx context.Context, v *Volume) error {
	if m.OnUpdate != nil {
		return m.OnUpdate(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) Get(ctx context.Context, appName string) (*Volume, error) {
	if m.OnGet != nil {
		return m.OnGet(ctx, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListByApp(ctx context.Context, appName string) ([]Volume, error) {
	if m.OnListByApp != nil {
		return m.OnListByApp(ctx, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListByFilter(ctx context.Context, f *Filter) ([]Volume, error) {
	if m.OnListByFilter != nil {
		return m.OnListByFilter(ctx, f)
	}
	return nil, nil
}

func (m *MockVolumeService) Delete(ctx context.Context, v *Volume) error {
	if m.OnDelete != nil {
		return m.OnDelete(ctx, v)
	}
	return nil
}

func (m *MockVolumeService) BindApp(ctx context.Context, opts *BindOpts) error {
	if m.OnBindApp != nil {
		return m.OnBindApp(ctx, opts)
	}
	return nil
}

func (m *MockVolumeService) UnbindApp(ctx context.Context, opts *BindOpts) error {
	if m.OnUnbindApp != nil {
		return m.OnUnbindApp(ctx, opts)
	}
	return nil
}

func (m *MockVolumeService) Binds(ctx context.Context, v *Volume) ([]VolumeBind, error) {
	if m.OnBinds != nil {
		return m.OnBinds(ctx, v)
	}
	return nil, nil
}

func (m *MockVolumeService) BindsForApp(ctx context.Context, v *Volume, appName string) ([]VolumeBind, error) {
	if m.OnBindsForApp != nil {
		return m.OnBindsForApp(ctx, v, appName)
	}
	return nil, nil
}

func (m *MockVolumeService) ListPlans(ctx context.Context) (map[string][]VolumePlan, error) {
	if m.OnListPlans != nil {
		return m.OnListPlans(ctx)
	}
	return nil, nil
}

func (m *MockVolumeService) CheckPoolVolumeConstraints(ctx context.Context, volume Volume) error {
	if m.OnCheckPoolVolumeConstraints != nil {
		return m.OnCheckPoolVolumeConstraints(ctx, volume)
	}
	return nil
}
