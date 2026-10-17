# Opt-in Android target compilation packages

Native source libraries accept:

```bp
target_package: {
    manifest: "lineage-ur-target/snapshots/HASH/package.json",
    sha256: "MANIFEST_SHA256",
},
```

The companion Lineage UR exporter prepares the manifest, compiled Android
objects, optional per-object ABI dumps, headers and provenance. Apply the
generated defaults to the original source module and set
`LINEAGE_USE_PACKAGED_TARGET_LIBS=true`.

Selection happens during compilation, after all target variations exist.
Matching packages return normal `Objects` and retain the library's object-reuse,
linking, header-export and installation providers. No source/prebuilt dependency
replacement mutator is involved, so a platform package cannot silently become
an APEX package. API stubs and unmatched variants compile as usual.

Hashes cover manifest contents, inventoried package files, source/build inputs
and recorded toolchain identities. All inspected files are Ninja regeneration
dependencies. Product context is taken from `AConfig()` so the original product
identity is available even when the module uses generic configuration semantics.
`cmd/target_package_context` computes the same hash from reference variables.
Output-directory references are normalized; device/CPU/API differences remain.

Tests: `TestPackagedTargetLibrary`, `TestPackagedTargetOverlaySyntax`,
`TestTargetPackageContext`, and `apex.TestPackagedTargetApexVariants`. The optional
overlay syntax test reads `LINEAGE_UR_TARGET_TEST_OVERLAY`.

The exporter covers six C libraries plus four Skia/Perfetto libraries from an
r8q reference. It preserves ARM64/ARM32 ThinLTO bitcode and native ARM objects.
The graphics/tracing bundle contains 18 variants and 10,916 compilation objects;
its source overlays contain multiple library defaults in each component.
Standalone ARM64/Bionic checks exercise CPU canvas rendering and in-process
C/C++ tracing. Generated reference outputs are provenance; the exporter's
source checksum guards include recorded headers and generator input leaves.
This is experimental local code;
full ROM and physical-device validation remain outstanding. The source tree
is retained for checked inputs and fallback. See the companion
`lineage/target-libraries/README.md` for exact commands and compatibility limits.
