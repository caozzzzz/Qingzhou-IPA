# Upstream integration

Pinned source: majd/ipatool d9c0247daea564d3ec7d04a9b015d0a29af66b04 (2026-10-25).
Upstream archive SHA-256: 13c10c4021fefc9614af97c67bf43cd7cf79b255079f6697fe35d252d24c4f9f.

Includes macOS 27 ARM64 SAP services and framework-backed Store operations (authentication, purchase, query, download), preserving Qingzhou session management, response diagnostics, cancellation and iOS native network adapter.

Upstream #642 remains unresolved: no claim that adopting main alone restores real-account authentication. Full iOS license acquisition, version queries and downloads must be validated with a real account on the target device.

## Additional reviewed compatibility changes

- #635: implement both NSURLComponents query setters with separate decoded/percent-encoded semantics and nil-query clearing, exercised via native guest selectors.
- #634: narrowly recover the actual server authentication plist after the known unsupported AMSError import. Only recognizable credential/dialog responses pass to the existing strict parser; unknown runtime failures still fail. Server account dialogs are explicit failures, never successful sessions.
- Keep iOS URLSession authentication transport available inside the new Store backend and retain Go-owned cookies and redirect validation.
- Preserve iOS sandbox cache override and cancellation context propagation.
- New ARM64 resource cache is separate from legacy x86 resources; interpreter build explicitly selects aarch64 and checks support before tests.
