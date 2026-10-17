package cc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/blueprint/parser"
)

func TestPackagedTargetOverlaySyntax(t *testing.T) {
	root := os.Getenv("LINEAGE_UR_TARGET_TEST_OVERLAY")
	if root == "" {
		t.Skip("set LINEAGE_UR_TARGET_TEST_OVERLAY to a prepared target overlay")
	}
	count := 0
	adapters := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "Android.bp" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		parsed, errors := parser.ParseAndEval(path, file, parser.NewScope(nil))
		if len(errors) != 0 {
			t.Fatalf("%s: %v", path, errors)
		}
		for _, definition := range parsed.Defs {
			module, ok := definition.(*parser.Module)
			if !ok {
				continue
			}
			seen := map[string]bool{}
			for _, property := range module.Properties {
				if seen[property.Name] {
					t.Fatalf("%s: duplicate %s property", path, property.Name)
				}
				seen[property.Name] = true
				if module.Type == "cc_defaults" && property.Name == "name" {
					if name, ok := property.Value.(*parser.String); ok && strings.HasPrefix(name.Value, "lineage_ur_target_") {
						if adapters[name.Value] {
							t.Fatalf("duplicate generated defaults: %s", name.Value)
						}
						adapters[name.Value] = true
					}
				}
			}
		}
		if strings.Contains(path, "lineage-ur-target") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(root), "inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Libraries []struct {
			Repository string `json:"repository"`
			Module     string `json:"module"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(raw, &inventory); err != nil {
		t.Fatal(err)
	}
	repositories := map[string]bool{}
	for _, library := range inventory.Libraries {
		repositories[library.Repository] = true
		if !adapters["lineage_ur_target_"+library.Module] {
			t.Fatalf("missing defaults for %s", library.Module)
		}
	}
	if count != len(repositories) || count == 0 {
		t.Fatalf("expected %d component adapter files; got %d", len(repositories), count)
	}
	if len(adapters) != len(inventory.Libraries) {
		t.Fatalf("expected %d defaults declarations; got %d", len(inventory.Libraries), len(adapters))
	}
}
