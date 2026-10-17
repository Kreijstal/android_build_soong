package cc

import (
	"encoding/json"
	"strings"
	"testing"

	"android/soong/android"
)

func TestPackagedTargetLibrary(t *testing.T) {
	for _, scenario := range []string{"enabled", "disabled", "missing", "corrupt", "changed-source", "context", "variant", "environment"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := targetPackageManifest{
				Schema: 1, Module: "libexample", Context: "unused",
				Files:    map[string]string{"example.o": targetPackageHash([]byte("compiled object"))},
				Inputs:   map[string]string{"example.c": targetPackageHash([]byte("int answer(void) { return 42; }"))},
				Variants: []targetPackageVariant{{Variation: "android_arm64_armv8-a_static", Objects: []string{"example.o"}}},
			}
			fs := android.MockFS{"example.c": []byte("int answer(void) { return 42; }"), "pkg/example.o": []byte("compiled object")}
			if scenario == "missing" {
				delete(fs, "pkg/example.o")
			}
			if scenario == "corrupt" {
				fs["pkg/example.o"] = []byte("corrupted object")
			}
			if scenario == "changed-source" {
				fs["example.c"] = []byte("int answer(void) { return 7; }")
			}
			if scenario == "variant" {
				manifest.Variants[0].Variation = "android_arm64_armv9-a_static"
			}
			if scenario == "environment" {
				manifest.Environment = map[string]string{"THINLTO_USE_MLGO": "different"}
			}
			optIn := "true"
			if scenario == "disabled" {
				optIn = "false"
			}
			result := android.GroupFixturePreparers(
				prepareForCcTest,
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_LIBS": optIn}),
				android.FixtureModifyContext(func(ctx *android.TestContext) {
					manifest.Context = android.TargetPackageContext(ctx.Config().ProductVariables(), ctx.Config().OutDir())
					if scenario == "context" {
						manifest.Context = "different product"
					}
				}),
				android.FixtureModifyMockFS(func(inputs android.MockFS) {
					for name, data := range fs {
						inputs[name] = data
					}
					raw, err := json.Marshal(manifest)
					if err != nil {
						t.Fatal(err)
					}
					inputs["pkg/package.json"] = raw
					inputs["Android.bp"] = []byte(`
                        cc_library { name: "libexample", host_supported: true, srcs: ["example.c"], stl: "none",
                            target_package: { manifest: "pkg/package.json", sha256: "` + targetPackageHash(raw) + `" },
                        }
                        cc_binary { name: "consumer", stl: "none", shared_libs: ["libexample"] }
                    `)
				}),
			).RunTest(t)
			module := result.ModuleForTests(t, "libexample", "android_arm64_armv8-a_static")
			info := android.OtherModuleProviderOrDefault(result.TestContext, module.Module(), TargetPackageInfoProvider)
			selected := scenario == "enabled"
			if (info.Objects > 0) != selected {
				t.Fatalf("used target package = %v, want %v", info.Objects > 0, selected)
			}
			if (module.MaybeOutput("obj/example.o").Rule == nil) != selected {
				t.Fatal("source compile action does not agree with package selection")
			}
			shared := result.ModuleForTests(t, "libexample", "android_arm64_armv8-a_shared")
			link := shared.Rule("ld")
			if strings.Contains(link.Args["inCommaList"], "pkg/example.o") {
				t.Fatal("unexpected archive input syntax")
			}
			if selected && !strings.Contains(strings.Join(link.Inputs.Strings(), " "), "pkg/example.o") {
				t.Fatalf("shared library did not reuse packaged static object: %v", link.Inputs)
			}
			host := result.Config.BuildOS.String() + "_" + result.Config.BuildArch.String() + "_static"
			hostInfo := android.OtherModuleProviderOrDefault(result.TestContext, result.ModuleForTests(t, "libexample", host).Module(), TargetPackageInfoProvider)
			if hostInfo.Objects != 0 {
				t.Fatal("Android object package selected for host")
			}

		})
	}
}
