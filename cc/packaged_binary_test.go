package cc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"android/soong/android"
	"github.com/google/blueprint/proptools"
)

func TestPackagedSharedTargetLibrary(t *testing.T) {
	for _, scenario := range []string{"selected", "disabled", "missing-driver", "missing-dependency", "corrupt-dependency", "bad-manifest-hash", "api", "arch", "partition", "source-sanitized"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := binaryPackageManifest{
				Schema: 1, Arch: "arm64", AndroidAPI: 36, Partition: "vendor",
				Files: map[string]string{
					"driver.so":     targetPackageHash([]byte("driver")),
					"dependency.so": targetPackageHash([]byte("dependency")),
				},
				Libraries: map[string]string{"libexample": "driver.so", "libexample_dependency": "dependency.so"},
			}
			fs := android.MockFS{"source.c": nil, "adapters/pkg/driver.so": []byte("driver"), "adapters/pkg/dependency.so": []byte("dependency")}
			switch scenario {
			case "missing-driver":
				delete(fs, "adapters/pkg/driver.so")
			case "missing-dependency":
				delete(fs, "adapters/pkg/dependency.so")
			case "corrupt-dependency":
				fs["adapters/pkg/dependency.so"] = []byte("modified")
			case "api":
				manifest.AndroidAPI = 35
			case "arch":
				manifest.Arch = "x86_64"
			case "partition":
				manifest.Partition = "system"
			}
			raw, _ := json.Marshal(manifest)
			checksum := targetPackageHash(raw)
			if scenario == "bad-manifest-hash" {
				checksum = "incorrect"
			}
			fs["adapters/pkg/package.json"] = raw
			fs["adapters/Android.bp"] = []byte(fmt.Sprintf(`
                cc_packaged_library_shared_target {
                    name: "libexample", vendor: true, stl: "none",
                    srcs: ["pkg/driver.so"], shared_libs: ["libexample_dependency"],
                    package_manifest: "pkg/package.json", package_sha256: %q,
                }
                cc_packaged_library_shared_target {
                    name: "libexample_dependency", vendor: true, stl: "none",
                    srcs: ["pkg/dependency.so"],
                    package_manifest: "pkg/package.json", package_sha256: %q,
                }
            `, checksum, checksum))
			optIn := "true"
			if scenario == "disabled" {
				optIn = ""
			}
			sanitizer := ""
			if scenario == "source-sanitized" {
				sanitizer = "sanitize: { integer_overflow: true },"
			}
			result := android.GroupFixturePreparers(
				prepareForPrebuiltTest,
				android.PrepareForTestWithLicenses,
				android.PrepareForTestWithLicenseDefaultModules,
				android.FixtureModifyProductVariables(func(v android.FixtureProductVariables) {
					api := 36
					v.Platform_sdk_version = &api
					v.Platform_sdk_final = proptools.BoolPtr(true)
					v.Platform_sdk_codename = proptools.StringPtr("REL")
				}),
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_BINARIES": optIn}),
				fs.AddToFixture(),
			).RunTestWithBp(t, fmt.Sprintf(`
                cc_library_shared { name: "libexample", vendor: true, stl: "none", srcs: ["source.c"], %s }
                cc_binary { name: "consumer", vendor: true, stl: "none", shared_libs: ["libexample"] }
            `, sanitizer))
			source := result.ModuleForTests(t, "libexample", "android_vendor_arm64_armv8-a_shared").Module().(*Module)
			want := scenario == "selected"
			if source.IsReplacedByPrebuilt() != want {
				t.Fatalf("replacement = %v, want %v", source.IsReplacedByPrebuilt(), want)
			}
			consumer := result.ModuleForTests(t, "consumer", "android_vendor_arm64_armv8-a").Module()
			chosen := source
			if want {
				chosen = result.ModuleForTests(t, "prebuilt_libexample", "android_vendor_arm64_armv8-a_shared").Module().(*Module)
			}
			if !android.HasDirectDep(result.TestContext, consumer, chosen) {
				t.Fatal("consumer did not select the expected shared library")
			}
			arm := result.ModuleForTests(t, "libexample", "android_vendor_arm_armv7-a-neon_shared").Module().(*Module)
			if arm.IsReplacedByPrebuilt() {
				t.Fatal("ARM64 package replaced ARM32 source")
			}
		})
	}
}

func TestPackagedSharedTargetSwiftShader(t *testing.T) {
	root := os.Getenv("LINEAGE_UR_SWIFTSHADER_PACKAGE")
	if root == "" {
		t.Skip("set LINEAGE_UR_SWIFTSHADER_PACKAGE to the generated Android package")
	}
	base := android.MockFS{"source.c": nil}
	kinds := ""
	for _, name := range []string{"BSD", "MIT", "NCSA", "LGPL-2.1+", "GPL-3.0+"} {
		kinds += fmt.Sprintf("license_kind { name: %q }\n", "SPDX-license-identifier-"+name)
	}
	base["license-kinds/Android.bp"] = []byte(kinds)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base[filepath.Join("adapters", relative)] = raw
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"selected", "disabled", "corrupt-llvm"} {
		t.Run(scenario, func(t *testing.T) {
			fs := android.MockFS{}
			for name, raw := range base {
				fs[name] = raw
			}
			optIn := "true"
			if scenario == "disabled" {
				optIn = ""
			}
			if scenario == "corrupt-llvm" {
				for name := range fs {
					if strings.HasSuffix(name, "/liblineage_ur_llvm21.so") {
						fs[name] = []byte("corrupt")
					}
				}
			}
			result := android.GroupFixturePreparers(
				prepareForPrebuiltTest,
				android.PrepareForTestWithLicenses,
				android.PrepareForTestWithLicenseDefaultModules,
				android.FixtureModifyProductVariables(func(v android.FixtureProductVariables) {
					api := 36
					v.Platform_sdk_version = &api
					v.Platform_sdk_final = proptools.BoolPtr(true)
					v.Platform_sdk_codename = proptools.StringPtr("REL")
				}),
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_BINARIES": optIn}),
				fs.AddToFixture(),
			).RunTestWithBp(t, `
                cc_library_shared { name: "vulkan.pastel", vendor: true, stl: "none", srcs: ["source.c"], relative_install_path: "hw" }
                cc_library_shared { name: "libnativewindow", vendor_available: true, stl: "none", }
                cc_library_shared { name: "libhardware", vendor_available: true, stl: "none", }
                cc_library_shared { name: "libsync", vendor_available: true, stl: "none", }
                cc_library_shared { name: "liblog", vendor_available: true, stl: "none", }
                cc_binary { name: "consumer", vendor: true, stl: "none", shared_libs: ["vulkan.pastel"] }
            `)
			source := result.ModuleForTests(t, "vulkan.pastel", "android_vendor_arm64_armv8-a_shared").Module().(*Module)
			want := scenario == "selected"
			if source.IsReplacedByPrebuilt() != want {
				t.Fatalf("replacement = %v, want %v", source.IsReplacedByPrebuilt(), want)
			}
			consumer := result.ModuleForTests(t, "consumer", "android_vendor_arm64_armv8-a").Module()
			chosen := source
			if want {
				chosen = result.ModuleForTests(t, "prebuilt_vulkan.pastel", "android_vendor_arm64_armv8-a_shared").Module().(*Module)
			}
			if !android.HasDirectDep(result.TestContext, consumer, chosen) {
				t.Fatal("generated package was not selected as expected")
			}
		})
	}
}

// Exercise the emitted adapters with the real independently built Termux ELFs
// and exported Android headers. Source, static and ARM32 variants remain present.
func TestPackagedSharedTargetTermuxLibraries(t *testing.T) {
	root := os.Getenv("LINEAGE_UR_TERMUX_LIBRARIES")
	if root == "" {
		t.Skip("set LINEAGE_UR_TERMUX_LIBRARIES to the generated adapter directory")
	}
	base := android.MockFS{"source.c": nil}
	kinds := ""
	for _, name := range []string{"Zlib", "BSD", "Libpng"} {
		kinds += fmt.Sprintf("license_kind { name: %q }\n", "SPDX-license-identifier-"+name)
	}
	base["license-kinds/Android.bp"] = []byte(kinds)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			base[filepath.Join("adapters", relative)] = raw
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"selected", "disabled", "corrupt-zlib", "corrupt-header", "source-sanitized"} {
		t.Run(scenario, func(t *testing.T) {
			fs := android.MockFS{}
			for name, raw := range base {
				fs[name] = raw
			}
			if scenario == "corrupt-zlib" {
				fs["adapters/package/lib64/libz.so"] = []byte("corrupt")
			}
			if scenario == "corrupt-header" {
				fs["adapters/package/include/libpng/png.h"] = []byte("corrupt")
			}
			optIn, sanitizer := "true", ""
			if scenario == "disabled" {
				optIn = ""
			}
			if scenario == "source-sanitized" {
				sanitizer = "sanitize: { integer_overflow: true },"
			}
			bp := ""
			for _, name := range []string{"libz", "libzstd", "libpng"} {
				bp += fmt.Sprintf(`cc_library { name: %q, vendor_available: true, stl: "none", srcs: ["source.c"], %s }`, name, sanitizer)
			}
			bp += `cc_binary { name: "consumer", vendor: true, stl: "none", srcs: ["source.c"], shared_libs: ["libz", "libzstd", "libpng"] }`
			result := android.GroupFixturePreparers(
				prepareForPrebuiltTest,
				android.PrepareForTestWithLicenses,
				android.PrepareForTestWithLicenseDefaultModules,
				android.FixtureModifyProductVariables(func(v android.FixtureProductVariables) {
					api := 36
					v.Platform_sdk_version = &api
					v.Platform_sdk_final = proptools.BoolPtr(true)
					v.Platform_sdk_codename = proptools.StringPtr("REL")
				}),
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_BINARIES": optIn}),
				fs.AddToFixture(),
			).RunTestWithBp(t, bp)
			consumer := result.ModuleForTests(t, "consumer", "android_vendor_arm64_armv8-a")
			for _, name := range []string{"libz", "libzstd", "libpng"} {
				source := result.ModuleForTests(t, name, "android_vendor_arm64_armv8-a_shared").Module().(*Module)
				want := scenario == "selected"
				if source.IsReplacedByPrebuilt() != want {
					t.Fatalf("%s replacement = %v, want %v", name, source.IsReplacedByPrebuilt(), want)
				}
				chosen := source
				if want {
					chosen = result.ModuleForTests(t, "prebuilt_"+name, "android_vendor_arm64_armv8-a_shared").Module().(*Module)
					flags := consumer.Rule("cc").Args["cFlags"]
					if !strings.Contains(flags, "-Iadapters/package/include/"+name) {
						t.Fatalf("consumer lost platform headers for %s: %s", name, flags)
					}
				}
				if !android.HasDirectDep(result.TestContext, consumer.Module(), chosen) {
					t.Fatalf("consumer did not select expected %s implementation", name)
				}
				for _, variant := range []string{"android_vendor_arm64_armv8-a_static", "android_vendor_arm_armv7-a-neon_shared", "android_arm64_armv8-a_shared"} {
					if result.ModuleForTests(t, name, variant).Module().(*Module).IsReplacedByPrebuilt() {
						t.Fatalf("%s: unsupported %s source variant replaced", name, variant)
					}
				}
			}
		})
	}
}
