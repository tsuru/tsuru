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
	OnAppend     func(string, string) error
	OnDelete     func(string) error
}

func (m *MockPlatformImageStorage) Upsert(ctx context.Context, name string) (*PlatformImage, error) {
	return m.OnUpsert(name)
}

func (m *MockPlatformImageStorage) FindByName(ctx context.Context, name string) (*PlatformImage, error) {
	return m.OnFindByName(name)
}

func (m *MockPlatformImageStorage) Append(ctx context.Context, name, image string) error {
	return m.OnAppend(name, image)
}

func (m *MockPlatformImageStorage) Delete(ctx context.Context, name string) error {
	return m.OnDelete(name)
}

// MockPlatformImageService implements PlatformImageService interface
type MockPlatformImageService struct {
	OnNewImage            func(string) (string, error)
	OnCurrentImage        func(string) (string, error)
	OnAppendImage         func(string, string) error
	OnDeleteImages        func(string) error
	OnListImages          func(string) ([]string, error)
	OnListImagesOrDefault func(string) ([]string, error)
	OnFindImage           func(string, string) (string, error)
}

func (m *MockPlatformImageService) NewImage(ctx context.Context, platformName string) (string, error) {
	if m.OnNewImage == nil {
		return "", nil
	}
	return m.OnNewImage(platformName)
}

func (m *MockPlatformImageService) CurrentImage(ctx context.Context, platformName string) (string, error) {
	if m.OnCurrentImage == nil {
		return "", nil
	}
	return m.OnCurrentImage(platformName)
}

func (m *MockPlatformImageService) AppendImage(ctx context.Context, platformName, imageID string) error {
	if m.OnAppendImage == nil {
		return nil
	}
	return m.OnAppendImage(platformName, imageID)
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

func (m *MockPlatformImageService) FindImage(ctx context.Context, platformName, image string) (string, error) {
	if m.OnFindImage == nil {
		return image, nil
	}
	return m.OnFindImage(platformName, image)
}
