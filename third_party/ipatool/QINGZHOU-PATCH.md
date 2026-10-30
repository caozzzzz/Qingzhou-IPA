Local modification to ipatool v2.7.0: expose actual remote IPA byte size and date source in GetVersionMetadataOutput. The size is obtained from the existing HTTP Range reader. Package file time is distinguished from an explicit releaseDate field. Authentication and download implementations are unchanged.


Additional Qingzhou modifications:
- Normalize and deduplicate numeric external version IDs as decimal strings.
- Add QingzhouListVersions to select existing Apple download, volumeStore compatibility and redownload requests. Only fixed Apple endpoints are accepted.
- Include pkg/appstore/qingzhou_version_sources_test.go and generated mock source for reproducible tests.
