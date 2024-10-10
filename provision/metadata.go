// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provision provides interfaces that need to be satisfied in order to
// implement a new provisioner on tsuru.

package provision

import (
	appTypes "github.com/tsuru/tsuru/types/app"
)

func GetAppMetadata(app *appTypes.App, process string) appTypes.Metadata {
	labels := map[string]string{}
	annotations := map[string]string{}

	for _, labelItem := range app.Metadata.Labels {
		labels[labelItem.Name] = labelItem.Value
	}
	for _, annotationItem := range app.Metadata.Annotations {
		annotations[annotationItem.Name] = annotationItem.Value
	}

	if process == "" {
		goto buildResult
	}

	for _, p := range app.Processes {
		if p.Name != process {
			continue
		}

		for _, labelItem := range p.Metadata.Labels {
			labels[labelItem.Name] = labelItem.Value
		}
		for _, annotationItem := range p.Metadata.Annotations {
			annotations[annotationItem.Name] = annotationItem.Value
		}
	}

buildResult:
	result := appTypes.Metadata{}

	for name, value := range labels {
		result.Labels = append(result.Labels, appTypes.MetadataItem{
			Name:  name,
			Value: value,
		})
	}

	for name, value := range annotations {
		result.Annotations = append(result.Annotations, appTypes.MetadataItem{
			Name:  name,
			Value: value,
		})
	}

	return result
}
