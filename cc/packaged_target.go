package cc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"

	"android/soong/android"
	"github.com/google/blueprint"
)

type targetPackageProperties struct {
	Target_package struct {
		Manifest string
		Sha256   string
	}
}

type targetPackageVariant struct {
	Variation string   `json:"variation"`
	Objects   []string `json:"objects"`
	AbiDumps  []string `json:"abi_dumps"`
}

type targetPackageManifest struct {
	Schema      int                    `json:"schema_version"`
	Module      string                 `json:"module"`
	Context     string                 `json:"context"`
	Files       map[string]string      `json:"files"`
	Inputs      map[string]string      `json:"inputs"`
	Environment map[string]string      `json:"environment"`
	Variants    []targetPackageVariant `json:"variants"`
}

type TargetPackageInfo struct {
	ManifestSha256 string
	Variation      string
	Objects        int
}

var TargetPackageInfoProvider = blueprint.NewProvider[TargetPackageInfo]()

func targetPackageHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func targetPackagePath(ctx ModuleContext, path string) android.OptionalPath {
	if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || strings.HasPrefix(path, "../") {
		return android.OptionalPath{}
	}
	return android.ExistentPathForSource(ctx, path)
}

func (library *libraryDecorator) packagedTargetObjects(ctx ModuleContext, flags Flags) (Objects, bool) {
	properties := library.targetPackage.Target_package
	if properties.Manifest == "" || !ctx.Config().IsEnvTrue("LINEAGE_USE_PACKAGED_TARGET_LIBS") ||
		ctx.Os() != android.Android || library.BuildStubs() || ctx.IsLlndk() || ctx.IsVendorPublicLibrary() ||
		flags.GcovCoverage || flags.EmitXrefs || flags.NeedTidyFiles ||
		ctx.Module().(*Module).isCoverageVariant() {
		return Objects{}, false
	}
	manifestPath := targetPackagePath(ctx, filepath.Join(ctx.ModuleDir(), properties.Manifest))
	if !manifestPath.Valid() {
		return Objects{}, false
	}
	// Validate once per manifest/configuration, not once per CPU/API/image
	// variant. All inspected files become Ninja regeneration dependencies.
	key := android.NewCustomOnceKey(struct{ Path, Hash string }{manifestPath.String(), properties.Sha256})
	manifest := ctx.Config().Once(key, func() interface{} {
		var result targetPackageManifest
		raw, err := android.ReadTargetPackageInput(ctx, manifestPath.Path())
		if err != nil || targetPackageHash(raw) != properties.Sha256 || json.Unmarshal(raw, &result) != nil ||
			result.Schema != 1 || result.Module != ctx.baseModuleName() || len(result.Files) == 0 || len(result.Inputs) == 0 ||
			result.Context != android.TargetPackageContext(ctx.AConfig().ProductVariables(), ctx.AConfig().OutDir()) {
			return (*targetPackageManifest)(nil)
		}
		for key, value := range result.Environment {
			if ctx.Config().Getenv(key) != value {
				return (*targetPackageManifest)(nil)
			}
		}
		for _, inventory := range []struct {
			Base  string
			Files map[string]string
		}{
			{filepath.Dir(manifestPath.String()), result.Files}, {"", result.Inputs},
		} {
			for file, checksum := range inventory.Files {
				if filepath.IsAbs(file) || filepath.Clean(file) != file || strings.HasPrefix(file, "../") {
					return (*targetPackageManifest)(nil)
				}
				path := targetPackagePath(ctx, filepath.Join(inventory.Base, file))
				if !path.Valid() {
					return (*targetPackageManifest)(nil)
				}
				raw, err := android.ReadTargetPackageInput(ctx, path.Path())
				if err != nil || targetPackageHash(raw) != checksum {
					return (*targetPackageManifest)(nil)
				}
			}
		}
		return &result
	}).(*targetPackageManifest)
	if manifest == nil {
		return Objects{}, false
	}
	for _, variant := range manifest.Variants {
		if variant.Variation != ctx.ModuleSubDir() {
			continue
		}
		if len(variant.Objects) == 0 || (library.sabi.shouldCreateSourceAbiDump() && len(variant.AbiDumps) == 0) {
			return Objects{}, false
		}
		paths := func(files []string) android.Paths {
			var paths android.Paths
			for _, file := range files {
				if _, exists := manifest.Files[file]; !exists {
					return nil
				}
				path := targetPackagePath(ctx, filepath.Join(filepath.Dir(manifestPath.String()), file))
				if !path.Valid() {
					return nil
				}
				paths = append(paths, path.Path())
			}
			return paths
		}
		objects := Objects{objFiles: paths(variant.Objects), sAbiDumpFiles: paths(variant.AbiDumps)}
		if len(objects.objFiles) != len(variant.Objects) || len(objects.sAbiDumpFiles) != len(variant.AbiDumps) {
			return Objects{}, false
		}
		library.reuseObjects = objects
		android.SetProvider(ctx, TargetPackageInfoProvider, TargetPackageInfo{properties.Sha256, variant.Variation, len(objects.objFiles)})
		return objects, true
	}
	return Objects{}, false
}
