// Copyright 2021 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	// Annotations max size according to
	// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/validation/objectmeta.go
	totalAnnotationSizeLimitB int = 256 * (1 << 10) // 256 kB
	tsuruPrefix                   = "tsuru.io/"
)

// Metadata represents the user defined labels and annotations
type Metadata struct {
	Labels      []MetadataItem `json:"labels"`
	Annotations []MetadataItem `json:"annotations"`
}

// MetadataItem is a Name-Value structure
type MetadataItem struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Delete bool   `json:"delete,omitempty" bson:"-"`
}

func (m *Metadata) Validate() error {
	errs := validateAnnotations(m.Annotations)
	errs.Append(validateLabels(m.Labels))
	if errs.Len() > 0 {
		return errs.ToError()
	}
	return nil
}

func (m Metadata) Annotation(v string) (string, bool) {
	return getItem(m.Annotations, v)
}

func (m Metadata) Label(v string) (string, bool) {
	return getItem(m.Labels, v)
}

func validateAnnotations(items []MetadataItem) *errors.MultiError {
	allErrs := errors.NewMultiError()
	fldPath := field.NewPath("metadata.annotations")
	var totalSize int64
	for _, item := range items {
		if item.Delete {
			continue
		}
		if strings.HasPrefix(item.Name, tsuruPrefix) {
			allErrs.Add(fmt.Errorf("prefix tsuru.io/ is private"))
		}
		for _, msg := range validation.IsQualifiedName(strings.ToLower(item.Name)) {
			allErrs.Add(field.Invalid(fldPath, item.Name, msg))
		}
		totalSize += (int64)(len(item.Name)) + (int64)(len(item.Value))
	}
	if totalSize > (int64)(totalAnnotationSizeLimitB) {
		allErrs.Add(field.TooLong(fldPath, "", totalAnnotationSizeLimitB))
	}
	return allErrs
}

func validateLabels(items []MetadataItem) *errors.MultiError {
	allErrs := errors.NewMultiError()
	fldPath := field.NewPath("metadata.labels")
	for _, item := range items {
		if item.Delete {
			continue
		}
		if strings.HasPrefix(item.Name, tsuruPrefix) {
			allErrs.Add(fmt.Errorf("prefix tsuru.io/ is private"))
		}
		for _, msg := range validation.IsQualifiedName(item.Name) {
			allErrs.Add(field.Invalid(fldPath, item.Name, msg))
		}
		for _, msg := range validation.IsValidLabelValue(item.Value) {
			allErrs.Add(field.Invalid(fldPath, item.Value, msg))
		}
	}
	return allErrs
}

func (m *Metadata) Update(new Metadata) {
	m.Annotations = updateList(m.Annotations, new.Annotations)
	m.Labels = updateList(m.Labels, new.Labels)
}

func updateList(list []MetadataItem, newItems []MetadataItem) []MetadataItem {
	for _, item := range newItems {
		n := hasItem(list, item.Name)
		if n != -1 {
			list = removeItem(list, n)
			if item.Delete {
				continue
			}
		}
		list = append(list, item)
	}
	return list
}

func hasItem(list []MetadataItem, name string) int {
	n := -1
	for i, v := range list {
		if v.Name == name {
			return i
		}
	}
	return n
}

func removeItem(list []MetadataItem, pos int) []MetadataItem {
	if pos == -1 {
		return list
	}
	list[pos] = list[len(list)-1]
	return list[:len(list)-1]
}

func getItem(items []MetadataItem, item string) (string, bool) {
	pos := hasItem(items, item)
	if pos == -1 {
		return "", false
	}
	return items[pos].Value, true
}
