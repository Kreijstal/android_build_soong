package java

import (
	"encoding/json"
	"fmt"
	"testing"

	"android/soong/android"
)

func TestAndroidPackagedApp(t *testing.T) {
	t.Parallel()
	const sourceBp = `
		android_app {
			name: "foo",
			srcs: ["a.java"],
			sdk_version: "current",
			product_specific: true,
		}
		android_test {
			name: "foo_tests",
			srcs: ["b.java"],
			sdk_version: "current",
			instrumentation_for: "foo",
		}
		android_app {
			name: "consumer",
			srcs: ["b.java"],
			sdk_version: "current",
			required: ["foo"],
		}
	`
	run := func(t *testing.T, fs android.MockFS, env map[string]string) *android.TestResult {
		return android.GroupFixturePreparers(
			prepareForJavaAndroidMkTest,
			android.FixtureMergeEnv(env),
			fs.AddToFixture(),
		).RunTestWithBp(t, sourceBp)
	}
	// Product configuration of the test fixture, as recorded by the exporter.
	probe := run(t, android.MockFS{}, nil)
	context := android.TargetPackageSourceContext(probe.Config.ProductVariables(), probe.Config.OutDir(),
		android.TargetPackageCheckoutDir())

	for _, scenario := range []string{"selected", "disabled", "corrupt-apk", "changed-input", "missing-input",
		"bad-manifest-hash", "context", "environment", "other-module", "stub-mode", "unrepresentable-input"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := packagedAppManifest{
				Schema: 1, Module: "foo", Context: context, Apk: "foo.apk",
				Files: map[string]string{"foo.apk": android.TargetPackageHash([]byte("apk")),
					"AndroidManifest.xml": android.TargetPackageHash([]byte("<manifest/>"))},
				AppManifest:       "AndroidManifest.xml",
				Inputs:            map[string]string{"a.java": android.TargetPackageHash([]byte("class A {}"))},
				Environment:       map[string]string{"RUN_ERROR_PRONE": ""},
				BuildFromTextStub: probe.Config.BuildFromTextStub(),
			}
			fs := android.MockFS{"a.java": []byte("class A {}"), "b.java": nil, "packaged/pkg/foo.apk": []byte("apk"),
				"packaged/pkg/AndroidManifest.xml": []byte("<manifest/>")}
			env := map[string]string{"LINEAGE_USE_PACKAGED_APPS": "true"}
			switch scenario {
			case "disabled":
				env = nil
			case "corrupt-apk":
				fs["packaged/pkg/foo.apk"] = []byte("modified")
			case "changed-input":
				fs["a.java"] = []byte("class A { int changed; }")
			case "missing-input":
				manifest.Inputs["missing.java"] = android.TargetPackageHash(nil)
			case "context":
				manifest.Context = "other"
			case "environment":
				env["RUN_ERROR_PRONE"] = "true"
			case "other-module":
				manifest.Module = "bar"
			case "unrepresentable-input":
				manifest.Inputs["$Generated$Name.java"] = android.TargetPackageHash(nil)
			case "stub-mode":
				manifest.BuildFromTextStub = !manifest.BuildFromTextStub
			}
			raw, _ := json.Marshal(manifest)
			checksum := android.TargetPackageHash(raw)
			if scenario == "bad-manifest-hash" {
				checksum = "incorrect"
			}
			fs["packaged/pkg/package.json"] = raw
			fs["packaged/Android.bp"] = []byte(fmt.Sprintf(`
				android_packaged_app {
					name: "foo",
					apk: "pkg/foo.apk",
					default_dev_cert: true,
					product_specific: true,
					overrides: ["OldFoo"],
					auto_generated_rros: ["foo__test__auto_generated_rro_vendor"],
					package_manifest: "pkg/package.json",
					package_sha256: %q,
				}
			`, checksum))
			result := run(t, fs, env)

			source := result.ModuleForTests(t, "foo", "android_common").Module().(*AndroidApp)
			want := scenario == "selected"
			if source.IsReplacedByPrebuilt() != want {
				t.Fatalf("replacement = %v, want %v", source.IsReplacedByPrebuilt(), want)
			}
			// Instrumentation tests compile against the app's classes, so they keep the source app.
			tests := result.ModuleForTests(t, "foo_tests", "android_common").Module()
			if !android.HasDirectDep(result.TestContext, tests, source) {
				t.Fatal("instrumentation test does not depend on the source app")
			}
			packaged := result.ModuleForTests(t, "prebuilt_foo", "android_common")
			if !want {
				return
			}
			// The packaged APK is re-signed with the default certificate and installed
			// where the source app would have been.
			signed := packaged.Output("signed/foo.apk")
			android.AssertStringEquals(t, "certificate",
				"build/make/target/product/security/testkey.x509.pem build/make/target/product/security/testkey.pk8",
				signed.Args["certificates"])
			app := packaged.Module().(*AndroidAppImport)
			android.AssertStringDoesContain(t, "install path", app.dexpreopter.installPath.String(),
				"/target/product/test_device/product/app/foo/foo.apk")
			// Source inputs are checked again at build time before the APK is used.
			check := packaged.Output("packaged-app/inputs.stamp")
			android.AssertPathsRelativeToTopEquals(t, "checked inputs", []string{"a.java"}, check.Implicits)
			uncompress := packaged.Output("jnis-uncompressed/prebuilt_foo.apk")
			android.AssertPathsRelativeToTopEquals(t, "input validation",
				[]string{check.Output.String()}, android.Paths{uncompress.Validation})
			// Auto-generated RROs read the app manifest through the replaced source module.
			android.AssertPathsRelativeToTopEquals(t, "manifest output",
				[]string{"packaged/pkg/AndroidManifest.xml"}, packaged.OutputFiles(result.TestContext, t, ".manifest.xml"))
			info := android.AndroidMkInfoForTest(t, result.TestContext, app)
			android.AssertDeepEquals(t, "overrides", []string{"OldFoo"}, info.PrimaryInfo.EntryMap["LOCAL_OVERRIDES_PACKAGES"])
			android.AssertStringListContains(t, "Make-only RRO requirement", info.PrimaryInfo.Required,
				"foo__test__auto_generated_rro_vendor")
		})
	}
}
