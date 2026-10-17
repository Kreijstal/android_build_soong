package apex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func TestPackagedTargetApexVariants(t *testing.T) {
	for _, apexSdk := range []string{"31", "32"} {
		t.Run(apexSdk, func(t *testing.T) {
			hash := func(raw []byte) string { result := sha256.Sum256(raw); return hex.EncodeToString(result[:]) }
			var context string
			result := android.GroupFixturePreparers(
				prepareForApexTest,
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_LIBS": "true"}),
				android.FixtureModifyContext(func(ctx *android.TestContext) {
					context = android.TargetPackageContext(ctx.Config().ProductVariables(), ctx.Config().OutDir())
				}),
				android.FixtureModifyMockFS(func(fs android.MockFS) {
					fs["example.c"] = []byte("int answer(void) { return 42; }")
					fs["pkg/core.o"], fs["pkg/apex.o"] = []byte("core object"), []byte("API 31 object")
					manifest := map[string]interface{}{
						"schema_version": 1, "module": "libexample", "context": context,
						"inputs": map[string]string{"example.c": hash(fs["example.c"])},
						"files":  map[string]string{"core.o": hash(fs["pkg/core.o"]), "apex.o": hash(fs["pkg/apex.o"])},
						"variants": []map[string]interface{}{
							{"variation": "android_arm64_armv8-a_static", "objects": []string{"core.o"}},
							{"variation": "android_arm64_armv8-a_static_apex31", "objects": []string{"apex.o"}},
						},
					}
					raw, err := json.Marshal(manifest)
					if err != nil {
						t.Fatal(err)
					}
					fs["pkg/package.json"] = raw
					fs["Android.bp"] = []byte(`
                        apex { name: "myapex", key: "myapex.key", native_shared_libs: ["libexample"], min_sdk_version: "` + apexSdk + `" }
                        apex_key { name: "myapex.key", public_key: "testkey.avbpubkey", private_key: "testkey.pem" }
                        cc_library { name: "libexample", srcs: ["example.c"], stl: "none", min_sdk_version: "29",
                            apex_available: ["//apex_available:platform", "myapex"],
                            target_package: { manifest: "pkg/package.json", sha256: "` + hash(raw) + `" },
                        }
                    `)
				}),
			).RunTest(t)
			for _, variant := range []string{"android_arm64_armv8-a_static", "android_arm64_armv8-a_static_apex" + apexSdk} {
				module := result.ModuleForTests(t, "libexample", variant)
				info := android.OtherModuleProviderOrDefault(result.TestContext, module.Module(), cc.TargetPackageInfoProvider)
				selected := !strings.Contains(variant, "apex32")
				if (info.Objects > 0) != selected {
					t.Fatalf("%s selection mismatch", variant)
				}
			}
			link := result.ModuleForTests(t, "libexample", "android_arm64_armv8-a_shared_apex"+apexSdk).Rule("ld")
			if apexSdk == "31" && !strings.Contains(strings.Join(link.Inputs.Strings(), " "), "pkg/apex.o") {
				t.Fatal("APEX did not link its own API-specific packaged object")
			}
		})
	}
}
