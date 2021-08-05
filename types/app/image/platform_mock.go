// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import "context"

var (
	_ PlatformImageStorage = &MockPlatformImageStorage{}
	_ PlatformImageService = &MockPlatformImageService{}
)

// MockPlatformStorage implements PlatformStorage interface
type MockPlatformImageStorage struct {
	OnUpsert     func(string) (*PlatformImage, error)
	OnFindByName func(string) (*PlatformImage, error)
	OnAppend     func(string, int, []string) error
	OnDelete     func(string) error
}

func (m *MockPlatformImageStorage) Upsert(ctx context.Context, name string) (*PlatformImage, error) {
	return m.OnUpsert(name)
}

func (m *MockPlatformImageStorage) FindByName(ctx context.Context, name string) (*PlatformImage, error) {
	return m.OnFindByName(name)
}

func (m *MockPlatformImageStorage) Append(ctx context.Context, name string, version int, images []string) error {
	return m.OnAppend(name, version, images)
}

func (m *MockPlatformImageStorage) Delete(ctx context.Context, name string) error {
	return m.OnDelete(name)
}

// MockPlatformImageService implements PlatformImageService interface
type MockPlatformImageService struct {
	OnNewVersion          func(string) (int, error)
	OnNewImage            func(ImageRegistry, string, int) string
	OnCurrentImage        func(ImageRegistry, string) (string, error)
	OnAppendImages        func(string, int, []string) error
	OnDeleteImages        func(string) error
	OnListImages          func(string) ([]string, error)
	OnListImagesOrDefault func(string) ([]string, error)
	OnFindImage           func(ImageRegistry, string, string) (string, error)
}

func (m *MockPlatformImageService) NewVersion(ctx context.Context, platformName string) (int, error) {
	if m.OnNewImage == nil {
		return 0, nil
	}
	return m.OnNewVersion(platformName)
}

func (m *MockPlatformImageService) NewImage(ctx context.Context, reg ImageRegistry, platformName string, version int) string {
	if m.OnNewImage == nil {
		return ""
	}
	return m.OnNewImage(reg, platformName, version)
}

func (m *MockPlatformImageService) CurrentImage(ctx context.Context, reg ImageRegistry, platformName string) (string, error) {
	if m.OnCurrentImage == nil {
		return "", nil
	}
	return m.OnCurrentImage(reg, platformName)
}

func (m *MockPlatformImageService) AppendImages(ctx context.Context, platformName string, version int, imageID []string) error {
	if m.OnAppendImages == nil {
		return nil
	}
	return m.OnAppendImages(platformName, version, imageID)
}

func (m *MockPlatformImageService) DeleteImages(ctx context.Context, platformName string) error {
	if m.OnDeleteImages == nil {
		return nil
	}
	return m.OnDeleteImages(platformName)
}

func (m *MockPlatformImageService) ListImages(ctx context.Context, platformName string) ([]string, error) {
	if m.OnListImages == nil {
		return []string{}, nil
	}
	return m.OnListImages(platformName)
}

func (m *MockPlatformImageService) ListImagesOrDefault(ctx context.Context, platformName string) ([]string, error) {
	if m.OnListImagesOrDefault == nil {
		return []string{"tsuru/" + platformName + ":latest"}, nil
	}
	return m.OnListImagesOrDefault(platformName)
}

func (m *MockPlatformImageService) FindImage(ctx context.Context, reg ImageRegistry, platformName, image string) (string, error) {
	if m.OnFindImage == nil {
		return image, nil
	}
	return m.OnFindImage(reg, platformName, image)
}
