# MeetSpace download verification

Checked on **23 October 2026**. The landing now links to Desktop **0.1.2-beta**
and Android **0.3.8-beta**. Client versions are independent from server versions.
The desktop repository has been renamed to `janickiy/meet-space-desktop`;
the former `meet-space-win` URL redirects there.

## Sources

- [Desktop release 0.1.2-beta](https://github.com/janickiy/meet-space-desktop/releases/tag/0.1.2-beta), pinned publication commit `7c5840267ad2c08cfbf1a97b99b1a30a64c98d82`.
- [Desktop catalog](https://github.com/janickiy/meet-space-desktop/blob/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/catalog.json) and [checksums](https://github.com/janickiy/meet-space-desktop/blob/7c5840267ad2c08cfbf1a97b99b1a30a64c98d82/source/SHA256SUMS).
- [Android release 0.3.8-beta](https://github.com/janickiy/meet-space-android/releases/tag/0.3.8-beta), pinned publication commit `86915851ca4ea7d0717ce4e98fdc9d11b2b0fda5`.
- [Android checksum](https://github.com/janickiy/meet-space-android/blob/86915851ca4ea7d0717ce4e98fdc9d11b2b0fda5/dist/MeetSpace-0.3.8-beta.apk.sha256).

Both GitHub Releases entries have empty assets arrays. Their release notes link
to actual binaries committed in Git: desktop packages are in `source/` via Git
LFS, Android in `dist/`. URLs in the landing are pinned to the publication
commits, not the mutable main branch. A source archive is not an installer.

## Published package records

| File | Architecture | Size, bytes | SHA-256 |
| --- | --- | ---: | --- |
| MeetSpace-0.1.2-beta-linux-arm64-test.AppImage | ARM64 | 119,067,319 | `7e008f306090dc1703fc2a27c37959bd12f15100791dc890479ea1886f6e84cc` |
| MeetSpace-0.1.2-beta-linux-x86_64-test.AppImage | x64 | 126,888,488 | `c0ac2540941a1066d1aac42ebf0a89b209714420fcecfa4af5180f3bc5ef9657` |
| MeetSpace-0.1.2-beta-linux-amd64-test.deb | x64 | 100,437,924 | `f316cfd52a06c088e69db6eb4acb1a100af833d4da2a1ef648edde554923abc1` |
| MeetSpace-0.1.2-beta-linux-arm64-test.deb | ARM64 | 95,662,188 | `dcd54dace5d4ffae837308e330db0e0ee30842f59f235b4e745ba7aeeb8091da` |
| MeetSpace-0.1.2-beta-linux-aarch64-test.rpm | ARM64 | 85,800,593 | `8e515daf5104cefe4ef2d8c4931bf14bdd91c1352f3ea19b34faf97b6c5fa4d1` |
| MeetSpace-0.1.2-beta-linux-x86_64-test.rpm | x64 | 90,079,817 | `258611790b851fa86aa9dead95df9e10a09cdfd1663ec1532ed87dd0a95397e8` |
| MeetSpace-0.1.2-beta-win-arm64-test-setup.exe | ARM64 | 116,494,316 | `38896ade79686783d7f9b169bafafa279c84062763e4150aee8875f23defdff9` |
| MeetSpace-0.1.2-beta-win-x64-test-setup.exe | x64 | 121,753,483 | `3b21846b1f73141743a1dd69829f48e3790332b77b3bef47b6c7cd909b83656e` |
| MeetSpace-0.3.8-beta.apk | Universal | 52,850,224 | `fac7bcdbbd307e10be611c3dd34b822f986fd4b8ed47a8ae45b66fbb1ad69ee2` |

## Verification

All eight desktop binary URLs returned HTTP 206 with expected Content-Range
total sizes. Initial bytes matched EXE/PE, DEB/ar, RPM, or AppImage/ELF.
The Git LFS SHA-256 object IDs and sizes matched the catalog and checksum list;
links also matched the published release notes. Full desktop binaries were
not downloaded or independently rehashed.

The Android APK was fully downloaded read-only (HTTP 200, 52,850,224 bytes)
and SHA-256 matched the publisher checksum. ZIP contents confirmed all four
native ABIs (arm64-v8a, armeabi-v7a, x86, x86_64). Gradle declares minSdk26,
versionCode26 and versionName0.3.8-beta. The APK v2 signing block and certificate
fingerprint match publisher metadata; no separate apksigner cryptographic
verification was performed.

Browser checks covered Windows x64/ARM64, Android universal, and Ubuntu,
Debian and Fedora on both Linux architectures. All nine selection states
showed the matching pinned URL, version and SHA-256. AppImage records remain
in the data catalog as before; the distribution selector offers DEB or RPM.
The production build and all 17 existing public-document checks passed.

## Compatibility and scope

Windows targets remain Windows10 22H2/Windows11 x64 and Windows11 ARM64,
confirmed by the new EXE metadata. Desktop apps remain **unsigned beta test
builds** with automatic updates disabled. A signature on the embedded server
installer does not sign the desktop application.

The latest Linux packages declare GLIBC>=2.35 and have static package checks.
Existing distro target labels remain unchanged; older 0.1.1 runtime results
must not be described as runtime acceptance for 0.1.2. Debian/Fedora ARM64
remain explicitly unconfirmed in the UI. No new cross-distribution claim
was added. Android remains a universal beta APK for Android8.0+ using its
existing development/beta certificate, not a store production release.

No installer or application was executed. Installation, upgrades and calls
were not tested. Server installer files, server release metadata and application
services were not changed. Website publication changes only shared download
records and canonical desktop repository links. Unrelated pending workspace
changes are excluded through an isolated build.

Evidence: `frontend/test-results/landing/download-update-audit.json`,
`download-update-ui.json`, `download-update-build.log` and
`published-download-update.jpg`. Audit source files and the isolated frontend
are in `/private/tmp/meetspace-download-update.nd2i_yfh`.
