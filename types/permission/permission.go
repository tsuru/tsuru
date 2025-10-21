// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	CtxGlobal          = ContextType("global")
	CtxApp             = ContextType("app")
	CtxJob             = ContextType("job")
	CtxTeam            = ContextType("team")
	CtxUser            = ContextType("user")
	CtxPool            = ContextType("pool")
	CtxService         = ContextType("service")
	CtxServiceInstance = ContextType("service-instance")
	CtxVolume          = ContextType("volume")
	CtxRouter          = ContextType("router")

	ContextTypes = []ContextType{
		CtxGlobal, CtxApp, CtxTeam, CtxUser, CtxPool, CtxService, CtxServiceInstance, CtxVolume, CtxRouter, CtxJob,
	}
)

type ContextType string

type PermissionContext struct {
	CtxType ContextType
	Value   string
}

type Permission struct {
	Scheme  *PermissionScheme
	Context PermissionContext
}

func (p *Permission) String() string {
	value := p.Context.Value
	if value != "" {
		value = " " + value
	}
	return fmt.Sprintf("%s(%s%s)", p.Scheme.FullName(), p.Context.CtxType, value)
}

type PermissionScheme struct {
	Name     string
	Parent   *PermissionScheme
	Contexts []ContextType
}

type PermissionSchemeList []*PermissionScheme

func (l PermissionSchemeList) Len() int           { return len(l) }
func (l PermissionSchemeList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l PermissionSchemeList) Less(i, j int) bool { return l[i].FullName() < l[j].FullName() }

func (s *PermissionScheme) nameParts() []string {
	parent := s
	var parts []string
	for parent != nil {
		if parent.Name != "" {
			parts = append(parts, parent.Name)
		}
		parent = parent.Parent
	}
	return parts
}

func (s *PermissionScheme) IsParent(other *PermissionScheme) bool {
	root := other
	myPointer := reflect.ValueOf(s).Pointer()
	for root != nil {
		if reflect.ValueOf(root).Pointer() == myPointer {
			return true
		}
		root = root.Parent
	}
	return false
}

func (s *PermissionScheme) Identifier() string {
	parts := s.nameParts()
	if len(parts) == 0 {
		return "All"
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(strings.Replace(cases.Title(language.English).String(parts[i]), "-", "", -1))
	}
	return b.String()
}

func (s *PermissionScheme) FullName() string {
	parts := s.nameParts()
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
		if i != 0 {
			b.WriteByte('.')
		}
	}
	return b.String()
}

func (s *PermissionScheme) AllowedContexts() []ContextType {
	contexts := []ContextType{CtxGlobal}
	if s.Contexts != nil {
		return append(contexts, s.Contexts...)
	}
	parent := s
	for parent != nil {
		if parent.Contexts != nil {
			return append(contexts, parent.Contexts...)
		}
		parent = parent.Parent
	}
	return contexts
}
