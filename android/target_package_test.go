package android

import "testing"

func TestTargetPackageContext(t *testing.T) {
	var config ProductVariables
	config.DeviceName = stringPtr("r8q")
	config.DeviceArchVariant = stringPtr("armv8-2a-dotprod")
	config.DexpreoptGlobalConfig = stringPtr("out/soong/dexpreopt.config")
	first := TargetPackageContext(config, "out")
	config.DexpreoptGlobalConfig = stringPtr("out-isolated/soong/dexpreopt.config")
	if first != TargetPackageContext(config, "out-isolated") {
		t.Fatal("moving OUT_DIR changed package context")
	}
	config.DeviceName = stringPtr("vsoc_arm64")
	if first == TargetPackageContext(config, "out-isolated") {
		t.Fatal("different device accepted the same context")
	}
	config.DeviceName = stringPtr("r8q")
	config.DeviceArchVariant = stringPtr("armv8-a")
	if first == TargetPackageContext(config, "out-isolated") {
		t.Fatal("different CPU feature baseline accepted")
	}
}

func TestTargetPackageSourceContext(t *testing.T) {
	var config ProductVariables
	config.DeviceName = stringPtr("r8q")
	config.VendorVars = map[string]map[string]string{"lineageVarsPlugin": {"KERNEL_MAKE_CMD": "/src/a/prebuilts/make"}}
	first := TargetPackageSourceContext(config, "out", "/src/a")
	config.VendorVars["lineageVarsPlugin"]["KERNEL_MAKE_CMD"] = "/src/b/prebuilts/make"
	if first != TargetPackageSourceContext(config, "out", "/src/b/") {
		t.Fatal("moving the checkout changed the app package context")
	}
	if first == TargetPackageSourceContext(config, "out", "/src/a") {
		t.Fatal("a path outside the checkout was accepted as normalized")
	}
	config.VendorVars["lineageVarsPlugin"]["KERNEL_MAKE_CMD"] = "/src/b/prebuilts/other"
	if first == TargetPackageSourceContext(config, "out", "/src/b") {
		t.Fatal("a different tool path accepted the same context")
	}
	// Values Soong derives after loading the file must not change the hash.
	config.Native_coverage = boolPtr(false)
	config.Build_from_text_stub = boolPtr(true)
	sdk := "36"
	config.Platform_sdk_version_or_codename = &sdk
	config.VendorVars["lineageVarsPlugin"]["KERNEL_MAKE_CMD"] = "/src/b/prebuilts/make"
	if first != TargetPackageSourceContext(config, "out", "/src/b") {
		t.Fatal("derived configuration values changed the app package context")
	}
}
