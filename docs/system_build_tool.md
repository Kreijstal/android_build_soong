# Opt-in system host tools (Lineage UR)

`system_build_tool` replaces a matching source-built host executable with a
compatibility-checked snapshot. Default builds continue to select the source.

```bp
system_build_tool {
    name: "example",
    src: "snapshots/example-HASH",
    licenses: ["appropriate_visible_license"],
    enabled: false,
    target: { linux_glibc_x86_64: { enabled: true } },
}
```

Keep the source module named `example` in the same namespace, with a matching
host architecture variant. Do not introduce another `prebuilt_example`.
`src` is relative to this module's directory. It must be a real snapshot
prepared before graph generation, not a path outside the Android tree.

The companion
[Lineage UR selector](https://github.com/Kreijstal/lineage-ur/blob/main/docs/system-tools.md)
resolves `PATH`, validates explicit version/feature policies, optionally enforces
an expected SHA-256, copies accepted executables into content-named snapshots,
and records provenance. Soong itself neither probes executables nor searches
the host filesystem for alternatives.

Set `LINEAGE_USE_SYSTEM_TOOLS=true` for selection. If unset/false, or the snapshot
does not exist, the normal source/prebuilt mutators retain the source module.
Soong tracks the environment variable and file existence for regeneration;
Ninja tracks the snapshot as a build input. No device variant is created.

The adapter exposes the normal host-tool provider to genrules, packages the tool
for sandbox consumers, installs `bin/<module>` and exports a host executable to
Make. Direct output-file references are also supported. Module names must match
the desired executable name. Custom stems, aliases, specialized Java/Go
providers, scripts, bootstrap tools and target libraries need separate work.
Source dependencies used by normal consumers are redirected, but explicit
source/checkbuild requests may still compile the original implementation.

This is local host-tool reuse, not a fully hermetic tool distribution. Native
runtime libraries are still supplied by the host. Missing runtime data, shared
libraries or incompatible behavior must be handled by a dedicated recipe and
consumer validation. Existing `prebuilt_build_tool` behavior is unchanged.

Focused Go tests: `TestSystemBuildTool` in `genrule`, and
`TestSystemBuildToolReplacesCcHostOnly` in `cc`. Run alongside `TestPrebuilts`
(`android`) and `TestPrebuiltTool` (`genrule`) to check default behavior.
