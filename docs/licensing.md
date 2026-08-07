# Licensing Policy

## Current repository

Unless a file or directory carries a different license notice, Roaminal's
first-party source code, tests, documentation, deployment manifests, and build
configuration in this repository are licensed under the Mozilla Public License
2.0 (`MPL-2.0`). The complete license is in the repository root `LICENSE` file.

Generated artifacts remain governed by the licenses of their corresponding
source files. Third-party components and assets retain their upstream licenses;
see `THIRD_PARTY_NOTICES.md` and the accompanying license files.

The canonical source repository is:

`https://github.com/ben-wangz/roaminal`

Distributors of executable builds, including container images, must comply with
MPL-2.0 section 3.2 by making the corresponding MPL-covered source available
and telling recipients how to obtain it. Builds from a tagged release or commit
should link to that same tag or commit in the canonical source repository.

## Plugin licensing boundary

Roaminal follows an open-core licensing model:

- The application core and modifications to its existing MPL-covered files are
  governed by MPL-2.0.
- A future public plugin SDK and plugin protocol will be licensed under
  Apache-2.0; the standard text is retained in
  [`LICENSES/Apache-2.0.txt`](../LICENSES/Apache-2.0.txt). Each such directory
  must contain its own `LICENSE` file and each source file must carry an
  `SPDX-License-Identifier: Apache-2.0` notice before it is published. Until
  those components exist, no current file is represented as Apache-2.0-licensed
  first-party code.
- Official Roaminal plugins are proprietary by default and must be distributed
  from separate repositories or artifacts under their own terms. They are not
  part of this repository and are not licensed by this repository's MPL-2.0
  license.
- Third-party plugin authors choose their own license, subject to the licenses
  of any code they copy or modify and any agreement governing distribution.

To preserve a clear boundary, plugins should integrate through a documented,
versioned protocol or SDK and should not copy or modify MPL-covered core source.
Separate repositories, build pipelines, packages, and release artifacts are the
preferred distribution model for proprietary plugins.

MPL-2.0 does not grant rights to the Roaminal name, logos, or other trademarks
except as required to describe the origin of the software and reproduce license
notices.

## Contributions

Contributions accepted into an MPL-covered part of this repository are made
under MPL-2.0 unless a separate written contribution agreement applies. Before
accepting outside contributions that may need to be offered under additional
commercial terms, the project should adopt a reviewed contributor license
agreement; this policy does not itself create one.
