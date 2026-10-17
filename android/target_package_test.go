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
