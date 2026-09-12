# Captured APT format contracts

These sanitized fixtures were captured on 2026-08-31 from the official
`debian:12`, `debian:13`, `ubuntu:22.04`, `ubuntu:24.04`, and `ubuntu:26.04`
container images after populating their package indexes. They preserve the
native C-locale output shape used by the collector while omitting unrelated
package and component records.

The policy fixtures deliberately include equal versions, exact-candidate
security/update ties, an epoch-bearing version, and a Debian security-only
candidate. Every HTTP source line in `policy.txt` has a matching binary
`Packages` target in `indextargets.txt`.

`ubuntu2404-multiarch` was captured on 2026-09-12 from an `ubuntu:24.04` host
with `dpkg --add-architecture i386` enabled and `libc6:i386` installed. It
exists because `apt-cache policy` prints a package header through
`pkgCache::PkgIterator::FullName(true)`, which omits the `:architecture`
qualifier for the native architecture and for `Architecture: all`, and appends
it for every foreign architecture. The fixture therefore contains all three
header spellings that a real host emits:

```text
adduser:          # Architecture: all, unqualified
libc6:            # native amd64, unqualified
libc6:i386:       # foreign architecture, qualified
```

Every single-architecture fixture happens to contain only unqualified headers
with unique package names, so none of them constrains how an unqualified header
is resolved. Keep this fixture registered in `targetFixtureDirectories`: without
it the parser can regress to resolving unqualified headers by package-name
uniqueness, which fails collection outright on any multi-arch host.

Capture commands:

```text
LC_ALL=C LANG=C apt-get indextargets
LC_ALL=C LANG=C dpkg-query --show --showformat=${binary:Package}|${Architecture}|${Version}|${db:Status-Status}\n ...
LC_ALL=C LANG=C apt-cache policy package:architecture ...
```
