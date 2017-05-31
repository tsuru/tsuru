// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fake

import (
	"errors"
	"fmt"

	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/builder"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/provision"
)

var _ builder.Builder = &FakeBuilder{}

type FakeBuilder struct {
	IsArchiveURLDeploy  bool
	IsArchiveFileDeploy bool
	IsRebuildDeploy     bool
}

func init() {
	builder.Register("fake", &FakeBuilder{})
}

func (b *FakeBuilder) Build(p provision.BuilderDeploy, app provision.App, evt *event.Event, opts builder.BuildOpts) (string, error) {
	if opts.BuildFromFile {
		return "", errors.New("build image from Dockerfile is not yet supported")
	}
	if opts.ArchiveFile != nil && opts.ArchiveSize != 0 {
		b.IsArchiveFileDeploy = true
	} else if opts.Rebuild {
		b.IsRebuildDeploy = true
	} else if opts.ArchiveURL != "" {
		b.IsArchiveURLDeploy = true
	} else {
		return "", errors.New("no valid files found")
	}
	buildingImage, err := image.AppNewBuilderImageName(app.GetName())
	if err != nil {
		return "", fmt.Errorf("error getting new image name for app %s", app.GetName())
	}
	err = image.AppendAppBuilderImageName(app.GetName(), buildingImage)
	if err != nil {
		return "", fmt.Errorf("unable to save image name. (%s)", err.Error())
	}
	imgHistorySize := image.ImageHistorySize()
	allImages, err := image.ListAppBuilderImages(app.GetName())
	if err != nil {
		log.Errorf("Couldn't list images for cleaning: %s", err)
		return "", nil
	}
	limit := len(allImages) - imgHistorySize
	if limit > 0 {
		for _, imgName := range allImages[:limit] {
			p.CleanImage(app.GetName(), imgName)
		}
	}
	return buildingImage, nil
}
