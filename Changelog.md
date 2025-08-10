# 0.2.5 / 2025-08-10

- add support for uploading specific subpaths

# 0.2.4 / 2025-08-09

- add some sftp tests
- ensure hashing takes into account file mode and size. modernize syntax

# 0.2.3 / 2025-08-06

- expose defaults concurrency and max chunk size
- reduce interface size for upload to read-only functions

# 0.2.2 / 2025-08-04

- preserve symlinks
- bump cli

# 0.2.1 / 2025-01-12

- add logging with log level flag
- configurable concurrency limits

# 0.2.0 / 2025-01-11

- add support for upload/download limits,
- improve download throughput with singleflight
- BREAKING: switch revision arg to flag and default to latest
- support chunking large files into multiple packs
- BREAKING: remove unnecessary readfile hook

# 0.1.3 / 2024-12-31

- ensure chunky works without cgo
- switch to a channel-based implementation
- consolidate and alias virt into repos

# 0.1.2 / 2024-12-26

- expose TagRevision method
- remove old code and formatting
- store tag history
- support programmatically listing tags

# 0.1.1 / 2024-11-17

- add support for specifying your own ignore and readfile functions in the uploader

# 0.1.0 / 2024-11-16

- add FindCommit
- Fix chunking algorithm
- Add checksum on unpacked data

# 0.0.9 / 2024-11-13

- improve programmatic interfaces
- go back to native ssh since that's faster
- internalize cache

# 0.0.8 / 2024-11-10

- support ssh remotes that don't include port
- support downloading from one remote location to another
- expose a public api
- create chunky client with upload and download.
- make the cache more generic

# 0.0.7 / 2024-11-10

- support using the underlying ssh cli for sftp

# 0.0.6 / 2024-11-10

- use external prompter
- remove cached files that have a different gob format

# 0.0.5 / 2024-11-04

- .gitignore -> .chunkyignore
- add a license and readme
- mark cat-\* and clean command as advanced

# 0.0.4 / 2024-11-04

- add tools for introspection
- simplify packfile using gob
- change commit codec to gob

# 0.0.3 / 2024-11-04

- add some color
- fix new cache bug
- support sync
- make prompt overridable
- remove unused code

# 0.0.2 / 2024-11-04

- pack system working
- support packing commits and compression
- added tagging, sftp support

# 0.0.1 / 2024-11-02

- initial commit
