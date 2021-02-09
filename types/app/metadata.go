// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
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

// Annotations max size according to
// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/validation/objectmeta.go
const totalAnnotationSizeLimitB int = 256 * (1 << 10) // 256 kB

func (m *Metadata) Validate() error {
	errs := validateAnnotations(m.Annotations)
	errs = append(errs, validateLabels(m.Labels)...)
	if len(errs) > 0 {
		errStr := ""
		for _, e := range errs {
			errStr = fmt.Sprintf("%s - %v\n", errStr, e)
		}
		return fmt.Errorf("metadata validation errors:\n%s", errStr)
	}
	return nil
}

func validateAnnotations(items []MetadataItem) field.ErrorList {
	allErrs := field.ErrorList{}
	fldPath := field.NewPath("metadata.annotations")
	var totalSize int64
	for _, item := range items {
		if item.Delete {
			continue
		}
		for _, msg := range validation.IsQualifiedName(strings.ToLower(item.Name)) {
			allErrs = append(allErrs, field.Invalid(fldPath, item.Name, msg))
		}
		totalSize += (int64)(len(item.Name)) + (int64)(len(item.Value))
	}
	if totalSize > (int64)(totalAnnotationSizeLimitB) {
		allErrs = append(allErrs, field.TooLong(fldPath, "", totalAnnotationSizeLimitB))
	}
	return allErrs
}

func validateLabels(items []MetadataItem) field.ErrorList {
	allErrs := field.ErrorList{}
	fldPath := field.NewPath("metadata.labels")
	for _, item := range items {
		if item.Delete {
			continue
		}
		for _, msg := range validation.IsQualifiedName(item.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath, item.Name, msg))
		}
		for _, msg := range validation.IsValidLabelValue(item.Value) {
			allErrs = append(allErrs, field.Invalid(fldPath, item.Value, msg))
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
			n = i
			break
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
