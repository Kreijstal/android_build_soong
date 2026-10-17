package apex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"android/soong/android"
)

func TestPackagedSharedTargetVendorApex(t *testing.T) {
	for _, scenario := range []string{"selected", "disabled", "missing-dependency"} {
		t.Run(scenario, func(t *testing.T) {
			hash := func(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
			manifest := map[string]interface{}{
				"schema_version": 1, "arch": "arm64", "android_api": 36, "partition": "vendor",
				"files":     map[string]string{"driver.so": hash([]byte("driver")), "dependency.so": hash([]byte("dependency"))},
				"libraries": map[string]string{"libexample": "driver.so", "libexample_dep": "dependency.so"},
			}
			raw, _ := json.Marshal(manifest)
			fs := android.MockFS{"source.c": nil, "pkg/package.json": raw, "pkg/driver.so": []byte("driver"), "pkg/dependency.so": []byte("dependency")}
			// The default vendor RRO fixture uses SDK 34 when the platform is 36.
			fs["prebuilts/sdk/34/public/android.jar"] = nil
			fs["prebuilts/sdk/34/public/framework.aidl"] = nil
			optIn := "true"
			if scenario == "disabled" {
				optIn = ""
			}
			if scenario == "missing-dependency" {
				delete(fs, "pkg/dependency.so")
			}
			result := android.GroupFixturePreparers(
				prepareForApexTest,
				android.FixtureModifyProductVariables(func(v android.FixtureProductVariables) {
					api := 36
					v.Platform_sdk_version = &api
				}),
				android.FixtureMergeEnv(map[string]string{"LINEAGE_USE_PACKAGED_TARGET_BINARIES": optIn}),
				fs.AddToFixture(),
			).RunTestWithBp(t, fmt.Sprintf(`
                apex { name: "myapex", key: "myapex.key", vendor: true, updatable: false, native_shared_libs: ["libexample"] }
                apex_key { name: "myapex.key", public_key: "testkey.avbpubkey", private_key: "testkey.pem" }
                cc_library_shared { name: "libexample", vendor: true, stl: "none", srcs: ["source.c"] }
                cc_packaged_library_shared_target {
                    name: "libexample", vendor: true, stl: "none", srcs: ["pkg/driver.so"],
                    package_manifest: "pkg/package.json", package_sha256: %q, shared_libs: ["libexample_dep"],
                }
                cc_packaged_library_shared_target {
                    name: "libexample_dep", vendor: true, stl: "none", srcs: ["pkg/dependency.so"],
                    package_manifest: "pkg/package.json", package_sha256: %q,
                }
            `, hash(raw), hash(raw)))
			contents := []string{"lib64/libexample.so"}
			if scenario == "selected" {
				contents = append(contents, "lib64/libexample_dep.so")
			}
			ensureExactContents(t, result.TestContext, "myapex", "android_common_myapex", contents)
		})
	}
}
