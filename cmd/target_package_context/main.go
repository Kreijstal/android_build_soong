package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"android/soong/android"
)

func main() {
	variables := flag.String("variables", "", "completed reference product variables JSON")
	out := flag.String("out-dir", "out", "reference OUT_DIR as used in the JSON")
	flag.Parse()
	raw, err := os.ReadFile(*variables)
	if err != nil {
		panic(err)
	}
	var config android.ProductVariables
	if err := json.Unmarshal(raw, &config); err != nil {
		panic(err)
	}
	fmt.Println(android.TargetPackageContext(config, *out))
}
