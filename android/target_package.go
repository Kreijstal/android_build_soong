package android

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

// ReadTargetPackageInput reads through Soong's source filesystem and makes
// content changes regenerate the graph. Graph tests use the same interface.
func ReadTargetPackageInput(ctx PathContext, path Path) ([]byte, error) {
	ctx.AddNinjaFileDeps(path.String())
	file, err := ctx.Config().fs.Open(path.String())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// TargetPackageUntrackedSourceSha256 hashes a checkout-relative source file
// without making it a graph regeneration dependency, which would overflow
// Ninja's deps log for very large input sets. Callers must check the file again
// in a build action. ok is false for missing or unreadable files.
func TargetPackageUntrackedSourceSha256(ctx PathContext, path string) (hash string, ok bool) {
	type result struct {
		hash string
		ok   bool
	}
	key := NewCustomOnceKey(struct{ Kind, Path string }{"target-package-untracked-input", path})
	value := ctx.Config().Once(key, func() interface{} {
		file, err := ctx.Config().fs.Open(path)
		if err != nil {
			return result{}
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			return result{}
		}
		return result{TargetPackageHash(raw), true}
	}).(result)
	return value.hash, value.ok
}

// TargetPackageHash returns the hex SHA-256 used by package manifests.
func TargetPackageHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// TargetPackageInputSha256 hashes one source input once per configuration.
// Packages share large input sets; each caller still records the file as a
// graph regeneration dependency.
func TargetPackageInputSha256(ctx PathContext, path Path) (string, error) {
	ctx.AddNinjaFileDeps(path.String())
	type result struct {
		hash string
		err  error
	}
	key := NewCustomOnceKey(struct{ Kind, Path string }{"target-package-input", path.String()})
	value := ctx.Config().Once(key, func() interface{} {
		raw, err := ReadTargetPackageInput(ctx, path)
		if err != nil {
			return result{err: err}
		}
		return result{hash: TargetPackageHash(raw)}
	}).(result)
	return value.hash, value.err
}

// TargetPackageContext hashes product configuration while allowing the output
// directory to move. It intentionally excludes neither product nor CPU/API data.
func TargetPackageContext(variables ProductVariables, outDir string) string {
	return targetPackageContext(variables, outDir, "")
}

// TargetPackageSourceContext also normalizes the checkout's absolute path, so a
// package from one checkout matches another checkout of the same product. Some
// product variables, such as Lineage's kernel build paths, embed that path.
//
// It also omits values Soong derives while loading the configuration, so the
// exporter's hash of the variables file equals the one computed in the build.
// Native_coverage and Platform_sdk_version_or_codename follow from hashed
// variables; callers compare Build_from_text_stub separately.
func TargetPackageSourceContext(variables ProductVariables, outDir, srcDir string) string {
	variables.Native_coverage = nil
	variables.Platform_sdk_version_or_codename = nil
	variables.Build_from_text_stub = boolPtr(false)
	return targetPackageContext(variables, outDir, srcDir)
}

// TargetPackageCheckoutDir is the absolute checkout path for TargetPackageSourceContext.
func TargetPackageCheckoutDir() string {
	return absSrcDir
}

func targetPackageContext(variables ProductVariables, outDir, srcDir string) string {
	if variables.Build_from_text_stub == nil {
		variables.Build_from_text_stub = boolPtr(false)
	}
	raw, err := json.Marshal(variables)
	if err != nil {
		panic(err)
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(err)
	}
	var normalize func(interface{}) interface{}
	normalize = func(v interface{}) interface{} {
		switch item := v.(type) {
		case string:
			if srcDir != "" {
				item = strings.ReplaceAll(item, strings.TrimSuffix(srcDir, "/")+"/", "$TOP/")
			}
			return strings.ReplaceAll(item, strings.TrimSuffix(outDir, "/")+"/", "$OUT/")
		case []interface{}:
			for i := range item {
				item[i] = normalize(item[i])
			}
		case map[string]interface{}:
			for k := range item {
				item[k] = normalize(item[k])
			}
		}
		return v
	}
	raw, err = json.Marshal(normalize(value))
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
