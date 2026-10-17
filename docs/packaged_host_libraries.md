# Optional packaged host C libraries

`cc_packaged_library_host` is the host-library counterpart to
`system_build_tool`. It accepts normal prebuilt library properties, exported
include directories and native link/header dependencies. A new `package_files`
property lists the complete transitive snapshot inventory.

```bp
cc_packaged_library_host {
    name: "libexample",
    compile_multilib: "64",
    stl: "none",
    unique_host_soname: false,
    static: { srcs: ["snapshot/lib/libexample.a"] },
    shared: { srcs: ["snapshot/lib/libexample.so"] },
    export_include_dirs: ["snapshot/include"],
    package_files: [
        "snapshot/package.json",
        "snapshot/lib/libexample.a",
        "snapshot/lib/libexample.so",
        "snapshot/include/example.h",
    ],
    licenses: ["appropriate_visible_license"],
    enabled: false,
    target: { linux_glibc_x86_64: { enabled: true } },
}
```

Set `LINEAGE_USE_PACKAGED_HOST_LIBS=true` to select a package instead of its
matching source module. Only Linux/glibc x86-64 host variants are eligible.
Disabled selection, missing snapshots, unavailable linkage variants or host
sanitization retain the source implementation. No device variants are created.

Package selection is deferred until native linkage variants and initial
configuration exist. The normal prebuilt providers then handle consumer
dependency replacement, headers, linking, installation and Make export.
Ordinary prebuilt types retain their existing selection timing.

The companion
[Lineage UR package preparer](https://github.com/Kreijstal/lineage-ur/blob/main/docs/host-libraries.md)
validates reviewed manifest/file/dependency hashes, architecture and SONAME,
stages real snapshot copies, and emits the module declarations. Missing files
are tracked through globs so their arrival/removal regenerates the graph. The
Soong module checks availability; checksum verification happens in the preparer.

This is opt-in reuse of reviewed C library builds, not automatic ABI detection
or a complete content-addressed build cache. Revalidate recipes when source,
toolchain or configuration changes. Target libraries, C++ ABI adaptation,
versioned SONAME layouts, download/publication infrastructure and fully isolated
host runtimes are outside this adapter's scope.

The companion preparer also accepts `unique_host_soname: true` for Android's
`<module>-host.so` naming, preserving the existing SONAME and dependency names.
The BoringSSL recipe exercises this with source-built shared `libssl` and
`libcrypto` (whose C++ runtime is already incorporated in the shared artifacts).
`TestPackagedHostLibraryGeneratedBoringSsl` checks the generated adapters when
`LINEAGE_UR_BORINGSSL_TEST_SNAPSHOT` names a prepared snapshot directory.

`TestPackagedHostLibraryGeneratedCLibraries` accepts a prepared six-library
overlay via `LINEAGE_UR_C_LIBRARIES_TEST_OVERLAY`. It checks static/shared
consumer dependencies, source fallback, unchanged device variants, PNG's zlib
dependency, and component-local license visibility. The companion recipes
cover bzip2, zlib, Expat, Zstandard, PNG and JPEG; legacy `libz_stable` stays on
its source path.
