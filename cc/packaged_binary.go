package cc

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"android/soong/android"
)

type binaryPackageProperties struct {
	Package_manifest string
	Package_sha256   string
}

type binaryPackageManifest struct {
	Schema     int               `json:"schema_version"`
	Arch       string            `json:"arch"`
	AndroidAPI int               `json:"android_api"`
	Partition  string            `json:"partition"`
	Files      map[string]string `json:"files"`
	Libraries  map[string]string `json:"libraries"`
}

// cc_packaged_library_shared_target selects a complete, hash-pinned vendor
// ARM64/Bionic shared-library package. Unsupported or incomplete packages keep
// the source implementation. Independent private dependencies use the same gate.
func PackagedSharedTargetLibraryFactory() android.Module {
	module, library := NewPrebuiltLibrary(android.DeviceSupported, "srcs")
	library.BuildOnlyShared()
	p := module.linker.(*prebuiltLibraryLinker)
	p.packagedTarget = true
	module.AddProperties(&p.binaryPackage)
	p.Prebuilt.ForcePrefer()
	return module.Init()
}

func binaryPackageRelative(path string) bool {
	return path != "" && path != "." && !filepath.IsAbs(path) &&
		filepath.Clean(path) == path && !strings.HasPrefix(path, "../")
}

func (p *prebuiltLibraryLinker) packagedTargetSrcs(ctx android.BaseModuleContext, candidate *Module) []string {
	if !ctx.Config().IsEnvTrue("LINEAGE_USE_PACKAGED_TARGET_BINARIES") ||
		ctx.Target().Os != android.Android || ctx.Target().Arch.ArchType != android.Arm64 ||
		!candidate.Enabled(ctx) || !candidate.InVendor() || candidate.InRecovery() ||
		candidate.InRamdisk() || candidate.InVendorRamdisk() || candidate.IsSdkVariant() ||
		!p.shared() || p.BuildStubs() || len(ctx.Config().SanitizeDevice()) != 0 ||
		ctx.DeviceConfig().ClangCoverageEnabled() || ctx.DeviceConfig().GcovCoverageEnabled() {
		return nil
	}
	for _, module := range []*Module{ctx.Module().(*Module), candidate} {
		if module.sanitize.Properties.SanitizerEnabled || module.isCoverageVariant() {
			return nil
		}
	}
	properties := p.binaryPackage
	if !binaryPackageRelative(properties.Package_manifest) {
		return nil
	}
	manifestPath := android.ExistentPathForSource(ctx, ctx.OtherModuleDir(candidate), properties.Package_manifest)
	if !manifestPath.Valid() {
		return nil
	}
	key := android.NewCustomOnceKey(struct{ Kind, Path, Hash string }{
		"binary-package", manifestPath.String(), properties.Package_sha256,
	})
	manifest := ctx.Config().Once(key, func() interface{} {
		var result binaryPackageManifest
		raw, err := android.ReadTargetPackageInput(ctx, manifestPath.Path())
		if err != nil || targetPackageHash(raw) != properties.Package_sha256 || json.Unmarshal(raw, &result) != nil ||
			result.Schema != 1 || result.Arch != "arm64" || result.Partition != "vendor" ||
			result.AndroidAPI <= 0 || ctx.Config().PlatformSdkVersion().FinalOrFutureInt() != result.AndroidAPI ||
			len(result.Files) == 0 || len(result.Libraries) == 0 {
			return (*binaryPackageManifest)(nil)
		}
		for file, checksum := range result.Files {
			if !binaryPackageRelative(file) {
				return (*binaryPackageManifest)(nil)
			}
			path := android.ExistentPathForSource(ctx, filepath.Dir(manifestPath.String()), file)
			if !path.Valid() {
				return (*binaryPackageManifest)(nil)
			}
			raw, err := android.ReadTargetPackageInput(ctx, path.Path())
			if err != nil || targetPackageHash(raw) != checksum {
				return (*binaryPackageManifest)(nil)
			}
		}
		// A partial library map must not permit selection of only part of the
		// dependency closure, even when the requested driver itself is present.
		for _, file := range result.Libraries {
			if !binaryPackageRelative(file) || result.Files[file] == "" {
				return (*binaryPackageManifest)(nil)
			}
		}
		return &result
	}).(*binaryPackageManifest)
	if manifest == nil {
		return nil
	}
	srcs := p.prebuiltSrcs(ctx)
	file := manifest.Libraries[candidate.BaseModuleName()]
	if len(srcs) != 1 || file == "" || srcs[0] != filepath.Join(filepath.Dir(properties.Package_manifest), file) {
		return nil
	}
	return srcs
}
