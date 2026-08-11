# Licensing Policy

Unless a file states otherwise, first-party source, tests, documentation,
deployment files, and build configuration are licensed under MPL-2.0. See the
root [`LICENSE`](../LICENSE). Third-party notices and license texts are in
[`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md) and [`LICENSES/`](../LICENSES/).

Distributors of executable builds, including container images, must provide the
corresponding MPL-covered source and identify the matching source tag or commit.
The canonical repository is `https://github.com/ben-wangz/roaminal`.

## Plugin boundary

- Core code and modifications to MPL-covered files remain MPL-2.0.
- A future public plugin SDK and protocol will use Apache-2.0, with its own
  license files and SPDX notices.
- Official plugins are proprietary by default and must be separate repositories
  or artifacts under their own terms.
- Third-party plugin authors choose their licenses, subject to copied code and
  distribution obligations.

MPL-2.0 does not grant trademark rights beyond identifying the project and
preserving notices. Contributions to MPL-covered code are accepted under MPL-2.0
unless a separate written agreement applies.
