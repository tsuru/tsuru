// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

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

func (m *MockPlatformImageStorage) Upsert(name string) (*PlatformImage, error) {
	return m.OnUpsert(name)
}

func (m *MockPlatformImageStorage) FindByName(name string) (*PlatformImage, error) {
	return m.OnFindByName(name)
}

func (m *MockPlatformImageStorage) Append(name, image string) error {
	return m.OnAppend(name, image)
}

func (m *MockPlatformImageStorage) Delete(name string) error {
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
}

func (m *MockPlatformImageService) NewImage(platformName string) (string, error) {
	if m.OnNewImage == nil {
		return "", nil
	}
	return m.OnNewImage(platformName)
}

func (m *MockPlatformImageService) CurrentImage(platformName string) (string, error) {
	if m.OnCurrentImage == nil {
		return "", nil
	}
	return m.OnCurrentImage(platformName)
}

func (m *MockPlatformImageService) AppendImage(platformName, imageID string) error {
	if m.OnAppendImage == nil {
		return nil
	}
	return m.OnAppendImage(platformName, imageID)
}

func (m *MockPlatformImageService) DeleteImages(platformName string) error {
	if m.OnDeleteImages == nil {
		return nil
	}
	return m.OnDeleteImages(platformName)
}

func (m *MockPlatformImageService) ListImages(platformName string) ([]string, error) {
	if m.OnListImages == nil {
		return []string{}, nil
	}
	return m.OnListImages(platformName)
}

func (m *MockPlatformImageService) ListImagesOrDefault(platformName string) ([]string, error) {
	if m.OnListImagesOrDefault == nil {
		return []string{"tsuru/" + platformName + ":latest"}, nil
	}
	return m.OnListImagesOrDefault(platformName)
}
