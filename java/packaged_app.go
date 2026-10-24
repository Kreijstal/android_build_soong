package java

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

type packagedAppProperties struct {
	// Hash-pinned package manifest, relative to this module's directory.
	Package_manifest *string
	Package_sha256   *string

	// Auto-generated RROs of the source app. They are required from Make only, as
	// the source app does, because each RRO depends on the source module itself.
	Auto_generated_rros []string
}

type packagedAppManifest struct {
	Schema      int               `json:"schema_version"`
	Module      string            `json:"module"`
	Context     string            `json:"context"`
	Apk         string            `json:"apk"`
	Files       map[string]string `json:"files"`
	Inputs      map[string]string `json:"inputs"`
	Environment map[string]string `json:"environment"`
	// Whether SDK stubs were built from API text files; `m --build-from-source-stub` changes it.
	BuildFromTextStub bool `json:"build_from_text_stub"`
	// The source app's merged manifest, provided as its ".manifest.xml" output
	// for the auto-generated RROs that reference it.
	AppManifest string `json:"app_manifest"`
}

// packagedAppRelative accepts clean checkout-relative paths that Soong can
// represent; anything else keeps the source app instead of a module error.
func packagedAppRelative(path string) bool {
	return path != "" && path != "." && !filepath.IsAbs(path) &&
		filepath.Clean(path) == path && !strings.HasPrefix(path, "../") && !strings.Contains(path, "$")
}

// packagedAppSrcs returns the packaged APK only while the opt-in flag is set and
// the manifest, its files, the recorded source inputs, product configuration and
// build environment all match. Anything else keeps the source android_app.
func packagedAppSrcs(ctx android.BaseModuleContext, a *AndroidAppImport) []string {
	if verifiedPackagedApp(ctx, a) == nil {
		return nil
	}
	return []string{a.properties.Apk.GetOrDefault(ctx, "")}
}

func verifiedPackagedApp(ctx android.BaseModuleContext, a *AndroidAppImport) *packagedAppManifest {
	properties := a.packagedApp
	if !ctx.Config().IsEnvTrue("LINEAGE_USE_PACKAGED_APPS") || !a.Enabled(ctx) {
		return nil
	}
	manifestFile, checksum := String(properties.Package_manifest), String(properties.Package_sha256)
	apk := a.properties.Apk.GetOrDefault(ctx, "")
	if !packagedAppRelative(manifestFile) || !packagedAppRelative(apk) {
		return nil
	}
	dir := ctx.OtherModuleDir(a)
	manifestPath := android.ExistentPathForSource(ctx, dir, manifestFile)
	if !manifestPath.Valid() {
		return nil
	}
	key := android.NewCustomOnceKey(struct{ Kind, Path, Hash string }{
		"packaged-app", manifestPath.String(), checksum,
	})
	manifest := ctx.Config().Once(key, func() interface{} {
		var result packagedAppManifest
		raw, err := android.ReadTargetPackageInput(ctx, manifestPath.Path())
		if err != nil || android.TargetPackageHash(raw) != checksum || json.Unmarshal(raw, &result) != nil ||
			result.Schema != 1 || result.Module != a.BaseModuleName() || result.Files[result.Apk] == "" ||
			(result.AppManifest != "" && result.Files[result.AppManifest] == "") ||
			len(result.Inputs) == 0 || result.BuildFromTextStub != ctx.Config().BuildFromTextStub() ||
			result.Context != android.TargetPackageSourceContext(ctx.Config().ProductVariables(), ctx.Config().OutDir(),
				android.TargetPackageCheckoutDir()) {
			return (*packagedAppManifest)(nil)
		}
		for key, value := range result.Environment {
			if ctx.Config().Getenv(key) != value {
				return (*packagedAppManifest)(nil)
			}
		}
		// Packaged files are graph dependencies, so replacing them re-runs selection.
		for file, expected := range result.Files {
			if !packagedAppRelative(file) {
				return (*packagedAppManifest)(nil)
			}
			path := android.ExistentPathForSource(ctx, filepath.Join(filepath.Dir(manifestPath.String()), file))
			if !path.Valid() {
				return (*packagedAppManifest)(nil)
			}
			actual, err := android.TargetPackageInputSha256(ctx, path.Path())
			if err != nil || actual != expected {
				return (*packagedAppManifest)(nil)
			}
		}
		// Tens of thousands of source inputs would overflow Ninja's deps log as graph
		// dependencies. They are checked here and again by packagedAppInputCheck
		// whenever one of them changes before the next graph generation.
		for file, expected := range result.Inputs {
			if !packagedAppRelative(file) {
				return (*packagedAppManifest)(nil)
			}
			if actual, ok := android.TargetPackageUntrackedSourceSha256(ctx, file); !ok || actual != expected {
				return (*packagedAppManifest)(nil)
			}
		}
		return &result
	}).(*packagedAppManifest)
	if manifest == nil || apk != filepath.Join(filepath.Dir(manifestFile), manifest.Apk) {
		return nil
	}
	return manifest
}

// android_packaged_app installs an APK packaged from a reference build of the
// same source app. It behaves as android_app_import: the build re-signs it with
// the configured certificate and runs dexpreopt against the current boot image.
// The source android_app with the same name remains the fallback.
func AndroidPackagedAppFactory() android.Module {
	module := &AndroidAppImport{}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.dexpreoptProperties)
	module.AddProperties(&module.usesLibrary.usesLibraryProperties)
	module.AddProperties(&module.packagedApp)
	module.populateAllVariantStructs()
	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		module.processVariants(ctx)
	})

	android.InitApexModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitPrebuiltModuleWithSrcSupplier(module, func(ctx android.BaseModuleContext, prebuilt android.Module) []string {
		return packagedAppSrcs(ctx, prebuilt.(*AndroidAppImport))
	}, "apk")
	module.prebuilt.ForcePrefer()

	module.usesLibrary.enforce = true

	return module
}

var packagedAppInputCheck = pctx.AndroidStaticRule("packagedAppInputCheck", blueprint.RuleParams{
	Command: `if ! sha256sum -c $in > $out.log 2>&1; then ` +
		`echo "packaged app $module: source inputs changed after Soong selected it;" ` +
		`"remove its lineage-ur-app adapter or unset LINEAGE_USE_PACKAGED_APPS" >&2; ` +
		`grep -v ': OK$$' $out.log | head -n 20 >&2; exit 1; fi && touch $out`,
	Description: "check packaged app inputs $module",
}, "module")

// packagedAppInputCheck verifies at build time the source inputs that are not
// graph dependencies. A change makes the build fail rather than ship the package.
func (a *AndroidAppImport) packagedAppInputValidation(ctx android.ModuleContext) android.Path {
	manifest := verifiedPackagedApp(ctx, a)
	if manifest == nil {
		return nil
	}
	files := android.SortedKeys(manifest.Inputs)
	var list strings.Builder
	inputs := make(android.Paths, 0, len(files))
	for _, file := range files {
		fmt.Fprintf(&list, "%s  %s\n", manifest.Inputs[file], file)
		inputs = append(inputs, android.PathForSource(ctx, file))
	}
	sums := android.PathForModuleOut(ctx, "packaged-app", "inputs.sha256")
	android.WriteFileRuleVerbatim(ctx, sums, list.String())
	stamp := android.PathForModuleOut(ctx, "packaged-app", "inputs.stamp")
	ctx.Build(pctx, android.BuildParams{
		Rule:      packagedAppInputCheck,
		Input:     sums,
		Implicits: inputs,
		Output:    stamp,
		Args:      map[string]string{"module": a.BaseModuleName()},
	})
	return stamp
}

// setPackagedAppOutputs provides the outputs other modules reference from the
// source app by tag.
func (a *AndroidAppImport) setPackagedAppOutputs(ctx android.ModuleContext) {
	manifest := verifiedPackagedApp(ctx, a)
	if manifest != nil && manifest.AppManifest != "" {
		dir := filepath.Dir(String(a.packagedApp.Package_manifest))
		ctx.SetOutputFiles(android.Paths{android.PathForModuleSrc(ctx, filepath.Join(dir, manifest.AppManifest))}, ".manifest.xml")
	}
}
