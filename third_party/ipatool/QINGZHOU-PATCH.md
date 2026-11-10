Local modification to ipatool v2.7.0: expose actual remote IPA byte size and date source in GetVersionMetadataOutput. The size is obtained from the existing HTTP Range reader. Package file time is distinguished from an explicit releaseDate field. Authentication and download implementations were subsequently updated as documented below.


Additional Qingzhou modifications:
- Normalize and deduplicate numeric external version IDs as decimal strings.
- Add QingzhouListVersions to select existing Apple download, volumeStore compatibility and redownload requests. Only fixed Apple endpoints are accepted.
- Include pkg/appstore/qingzhou_version_sources_test.go and generated mock source for reproducible tests.


iOS 0.4.0 protocol updates:
- Configurator 2.21 User-Agent; flat/nested dynamic URL bags; strict native authentication endpoint validation; account ID fallback from validated response header/cookie names.
- Native commerce request changes reference trikesh32/ipatool fix/native-store-download at 175c3e92a58a38fe538f4d94e785de4e575c6972 (MIT). Original upstream notices are retained.
- qingzhou_proof*.go adds an iOS C ABI provider for machine-scope Store proof, SP/RP handling and signed-payload replay; only two proof headers are accepted, only fixed Apple endpoints receive them. CDN requests are excluded.
- The provider calls the app's local WebKit device runtime. This combined iOS integration includes GPL-2.0-or-later components; see ios/COPYING and third_party/ipadn-store-runtime. The non-iOS provider does not implement Store proof.
