// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import "path/filepath"

func init() {
	RegisterModuleType("prebuilt_build_tool", NewPrebuiltBuildTool)
	RegisterModuleType("system_build_tool", NewSystemBuildTool)
}

type prebuiltBuildToolProperties struct {
	// Source file to be executed for this build tool
	Src *string `android:"path,arch_variant"`

	// Extra files that should trigger rules using this tool to rebuild
	Deps []string `android:"path,arch_variant"`

	// Create a make variable with the specified name that contains the path to
	// this prebuilt built tool, relative to the root of the source tree.
	Export_to_make_var *string
}

type prebuiltBuildTool struct {
	ModuleBase
	prebuilt Prebuilt

	properties prebuiltBuildToolProperties

	toolPath OptionalPath

	// Only the opt-in system_build_tool factory enables source fallback.
	systemTool bool
}

func (t *prebuiltBuildTool) Name() string {
	return t.prebuilt.Name(t.ModuleBase.Name())
}

func (t *prebuiltBuildTool) Prebuilt() *Prebuilt {
	return &t.prebuilt
}

func (t *prebuiltBuildTool) DepsMutator(ctx BottomUpMutatorContext) {
	if t.properties.Src == nil {
		ctx.PropertyErrorf("src", "missing prebuilt source file")
	}
}

func (t *prebuiltBuildTool) GenerateAndroidBuildActions(ctx ModuleContext) {
	if t.systemTool && !t.prebuilt.UsePrebuilt() {
		return
	}
	sourcePath := t.prebuilt.SingleSourcePath(ctx)
	installedPath := PathForModuleOut(ctx, t.BaseModuleName())
	deps := PathsForModuleSrc(ctx, t.properties.Deps)

	var fromPath = sourcePath.String()
	if !filepath.IsAbs(fromPath) {
		fromPath = "$$PWD/" + fromPath
	}

	ctx.Build(pctx, BuildParams{
		Rule:      Symlink,
		Output:    installedPath,
		Input:     sourcePath,
		Implicits: deps,
		Args: map[string]string{
			"fromPath": fromPath,
		},
	})
	if t.systemTool {
		// Preserve the normal host bin path for Make and direct-path users too.
		ctx.InstallFile(PathForModuleInstall(ctx, "bin"), t.BaseModuleName(), installedPath)
		ctx.SetOutputFiles(Paths{installedPath}, "")
		t.toolPath = OptionalPathForPath(installedPath)
		return
	}

	packagingDir := PathForModuleInstall(ctx, t.BaseModuleName())
	ctx.PackageFile(packagingDir, sourcePath.String(), sourcePath)
	for _, dep := range deps {
		ctx.PackageFile(packagingDir, dep.String(), dep)
	}

	t.toolPath = OptionalPathForPath(installedPath)
}

func (t *prebuiltBuildTool) MakeVars(ctx MakeVarsModuleContext) []ModuleMakeVarsValue {
	if t.systemTool && !t.prebuilt.UsePrebuilt() {
		return nil
	}
	if makeVar := String(t.properties.Export_to_make_var); makeVar != "" &&
		t.Target().Os == ctx.Config().BuildOS {
		return []ModuleMakeVarsValue{{makeVar, t.toolPath.String()}}
	}
	return nil
}

func (t *prebuiltBuildTool) HostToolPath() OptionalPath {
	return t.toolPath
}

var _ HostToolProvider = &prebuiltBuildTool{}

// prebuilt_build_tool is to declare prebuilts to be used during the build, particularly for use
// in genrules with the "tools" property.
func NewPrebuiltBuildTool() Module {
	module := &prebuiltBuildTool{}
	module.AddProperties(&module.properties)
	InitSingleSourcePrebuiltModule(module, &module.properties, "Src")
	InitAndroidArchModule(module, HostSupportedNoCross, MultilibFirst)
	return module
}

// system_build_tool selects a compatibility-checked, source-tree-local snapshot
// of a system host executable. The caller prepares the snapshot before Soong;
// graph generation never searches PATH or executes host probes. A matching
// source module is required for fallback. Device variants are never created.
//
// Existing prebuilt_build_tool behavior is intentionally unchanged.
type systemBuildTool struct {
	prebuiltBuildTool
}

func (t *systemBuildTool) AndroidMkEntries() []AndroidMkEntries {
	if !t.prebuilt.UsePrebuilt() {
		return nil
	}
	return []AndroidMkEntries{{
		Class:      "EXECUTABLES",
		OutputFile: t.toolPath,
		Include:    "$(BUILD_PREBUILT)",
	}}
}

func NewSystemBuildTool() Module {
	module := &systemBuildTool{prebuiltBuildTool: prebuiltBuildTool{systemTool: true}}
	module.AddProperties(&module.properties)
	InitPrebuiltModuleWithSrcSupplier(module, func(ctx BaseModuleContext, prebuilt Module) []string {
		t := prebuilt.(*systemBuildTool)
		if !t.Enabled(ctx) || !ctx.Config().IsEnvTrue("LINEAGE_USE_SYSTEM_TOOLS") {
			return nil
		}
		src := String(t.properties.Src)
		if src == "" {
			return nil
		}
		// Track missing files too, so adding/removing a snapshot regenerates
		// the graph and changes the source/prebuilt decision.
		if !ExistentPathForSource(ctx, ctx.OtherModuleDir(prebuilt), src).Valid() {
			return nil
		}
		return []string{src}
	}, "src")
	module.prebuilt.ForcePrefer()
	InitAndroidArchModule(module, HostSupportedNoCross, MultilibFirst)
	return module
}
