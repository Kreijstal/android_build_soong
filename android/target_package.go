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

// TargetPackageContext hashes product configuration while allowing the output
// directory to move. It intentionally excludes neither product nor CPU/API data.
func TargetPackageContext(variables ProductVariables, outDir string) string {
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
