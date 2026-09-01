# FileSystem image preview derivatives and disk cache

## Objective

Roaminal must avoid transferring the full remote image for the initial FileSystem
preview. The backend will generate and cache a lower-quality image derivative,
while retaining the source pixel dimensions and aspect ratio. The user can
explicitly load the original image, and downloads always return the untouched
remote file.

This document is the complete implementation contract. Remove it after all
acceptance criteria are met and permanent documentation is updated.

## Goals

- Reduce transfer size and repeated image conversion cost for FileSystem image
  previews.
- Keep the source width, height, aspect ratio, transparency, visual orientation,
  and supported animation. No resize, crop, or geometry change is allowed.
- Encode preview derivatives as lossy WebP at quality `75` and reduction effort
  `3`.
- Keep generated blobs in a bounded filesystem cache, never an in-memory cache.
- Keep image preview failures isolated from original viewing, downloads, other
  FileSystem viewers, and backend health.
- Preserve the current fit-to-viewport image layout.

## Non-goals

- Geometric thumbnails, responsive image sizes, cropping, image editing, or
  format selection by the user.
- Preview generation for video, PDF, SVG, or non-image content.
- Pre-generating derivatives while browsing the tree.
- Persisting derivatives as application state or writing anything to the remote
  host.
- Replacing browser-decoded image memory with compressed storage. Same-dimension
  WebP reduces network and cache bytes, but decoded browser memory is still based
  on pixel count.

## Current behavior

`frontend/src/filesystem/file-preview.tsx` loads image content through
`readContent` with `download=1`, so every initial preview transfers the complete
remote original. `backend/internal/server/filesystem_content_handlers.go`
streams source bytes and has no derivative or image cache concept. The backend
is built with `CGO_ENABLED=0`, the runtime image has no libvips installation,
and the Chart's `64Mi` `/tmp` volume is reserved for SSH multiplexing sockets.

## Chosen dependencies

### Image processing

Use `github.com/davidbyttow/govips/v2/vips` at `v2.18.0`, backed by libvips
`8.14` or newer.

libvips is selected for its low-memory, demand-driven pipeline and strong WebP
performance. The backend must use an explicit libvips startup/shutdown lifecycle
and must not initialize it independently per request.

WebP is the only derivative format in this version:

- Lossy WebP provides materially smaller photographic and mixed-content files
  than PNG while retaining alpha.
- PNG has no general-purpose lossy quality control. Palette quantization can
  reduce selected PNGs but is not a consistent replacement for this pipeline.
  PNG sources are therefore decoded and encoded as WebP; the remote PNG remains
  untouched.
- JPEG cannot retain alpha or animation.
- AVIF is not the default because its encoding latency and CPU cost are higher
  for an interactive, on-demand preview.

### Filesystem cache

Use `github.com/ydylla/fcache` at `v1.6.1`. It provides filesystem-backed blobs,
per-entry TTL, a target size, approximate LRU eviction, request coalescing, and
private file modes.

`fcache` treats expired entries as logical misses but does not guarantee prompt
physical deletion solely because TTL elapsed. Roaminal must add the janitor
defined below. Configure the library's eviction interval as zero so every
completed write schedules a capacity evaluation; do not retain its default
ten-minute capacity lag.

Update `THIRD_PARTY_NOTICES.md` and `LICENSES/` for govips (MIT), fcache
(Apache-2.0), and the dynamically linked libvips runtime (LGPL-2.1-or-later), as
required by the repository licensing policy.

## Backend boundaries

Add a dedicated internal image-preview service. It owns libvips lifecycle, the
conversion semaphore, source staging, `fcache`, cache-key construction, and the
expiration janitor. The HTTP server receives this service as an optional
dependency in the same construction graph as the FileSystem service.

The image-preview service must not know SSH commands, connection definitions, or
remote paths outside the source descriptor supplied by the FileSystem handler.
The handler remains responsible for authorization, root revision validation,
stat/open operations, and source consistency checks.

If image-preview initialization fails, Roaminal starts normally with derivative
generation disabled. The preview endpoint reports a retryable preview-unavailable
error and the frontend falls back to the original. `/healthz` is unaffected.

## Content API contract

Extend the existing authenticated endpoint:

`GET /api/v2/connection-instances/{connectionInstanceId}/filesystem/content`

The existing `path` and `rootRevision` parameters remain required by current
FileSystem behavior. Add the optional `variant` parameter:

| Query | Behavior |
| --- | --- |
| no `variant` | Preserve the current content-window and range behavior exactly. |
| `variant=preview` | Return the complete cached/generated WebP derivative for an eligible raster image. |
| `variant=original` | Return the complete original inline, without the existing preview-window truncation. |
| `download=1` | Return the complete remote original as an attachment, regardless of `variant`. |

Reject an unknown `variant` with HTTP `400` and
`filesystem_variant_invalid`. `download=1` is authoritative: it never returns a
derivative and never populates the image cache.

Both explicit variants support a single byte range using the existing range
rules. A preview range is evaluated against the generated WebP length. A full
preview response must not set `X-Roaminal-Content-Truncated`.

Response requirements:

| Response | Required headers |
| --- | --- |
| Preview | `Content-Type: image/webp`, exact `Content-Length`, variant-specific `ETag`, and `X-Roaminal-Image-Variant: preview` |
| Original view | Source MIME type, source `ETag`, and `X-Roaminal-Image-Variant: original`; no attachment disposition |
| Download | Source MIME type, source `ETag`, and the existing safe attachment disposition |
| Range | Existing `Accept-Ranges`, `Content-Range`, and `206` semantics |

The original ETag remains based on the source consistency token. The preview
ETag must include the full derivative identity digest and can never equal the
original ETag. Honor `If-None-Match` for both explicit variants after validating
the current root and source token.

`variant=preview` is allowed only for raster image MIME types supported by the
installed libvips loaders. SVG and formats without bounded raster dimensions are
not eligible. Use these errors:

| Condition | HTTP/code | Retryable |
| --- | --- | --- |
| Non-raster or unsupported source | `415 filesystem_image_preview_unsupported` | false |
| Decode, animation-preservation, or configured resource-limit rejection | `422 filesystem_image_preview_unavailable` | false |
| Temporary cache, staging, or converter failure | `503 filesystem_image_preview_unavailable` | true |
| Root/source changed during generation | Existing root-changed or content-unavailable contract | existing value |

Preview errors never return original bytes with an `image/webp` content type.
The frontend owns the automatic original fallback.

## Derivative identity

Construct a canonical, length-delimited identity containing all of the
following fields:

- connection instance ID;
- resolved root absolute path and root revision;
- normalized relative file path;
- source size and complete FileSystem consistency token;
- output format (`webp`), quality (`75`), and reduction effort (`3`);
- an explicit pipeline version, initially `filesystem-image-preview-v1`.

Hash the canonical identity with SHA-256. Use the complete digest for the
preview ETag and a deterministic 64-bit portion as the `fcache` key. Including
the connection instance and root prevents derivatives from crossing connection
or root boundaries. Any change to color handling, orientation, animation,
metadata, codec options, or safety behavior that can alter output must increment
the pipeline version.

## Conversion pipeline

On a preview cache miss, execute this sequence:

1. Validate the source entry is a file, has an eligible raster MIME type, and is
   within the source-byte limit before opening it.
2. Use `fcache.GetReaderOrPut` for per-key request coalescing. Only the elected
   filler acquires the conversion semaphore and contacts the remote source.
3. Stream the complete source to a private temporary file in the dedicated
   preview volume. Abort if the declared or observed byte limit is exceeded.
4. Compare the opened source entry with the original stat token and re-stat
   after staging. Reject the result if size, modification token, root revision,
   or path identity changed.
5. Load all pages/frames with govips. Validate dimensions, frame count, and total
   frame pixels before export.
6. Apply EXIF orientation to pixels and convert tagged color data to sRGB before
   removing metadata. This preserves browser-visible orientation and color even
   though EXIF and ICC metadata are stripped. A 90-degree orientation may swap
   width and height; this is orientation normalization, not resizing.
7. Do not call resize, thumbnail, crop, or resample operations. Export lossy
   WebP with quality `75`, reduction effort `3`, alpha retained, and unnecessary
   EXIF/XMP/comment metadata removed.
8. For a multi-frame source, retain every frame, frame order, delays, loop
   behavior, dimensions, and alpha. If the installed decoder/encoder cannot
   preserve the animation, fail the derivative instead of silently exporting
   only the first frame.
9. Reject output beyond the configured output-byte limit before committing it.
   A failed filler must leave no cache entry.
10. Close the image, remove the staging file in all paths, commit the WebP to
    `fcache`, and stream the returned cache reader to the client.

Generated WebP bytes may exist transiently in encoder memory, but no long-lived
byte-slice cache is permitted. Cache hits must stream from the filesystem reader.

## Resource limits

Initial defaults are fixed as follows and are configurable only through backend
deployment configuration, not the browser UI:

| Limit | Default |
| --- | ---: |
| Concurrent cache-miss conversions | `1` |
| Remote source bytes | `32 MiB` |
| Static image pixels | `100,000,000` |
| Animated frames | `200` |
| Animated total frame pixels | `200,000,000` |
| Encoded WebP bytes | `16 MiB` |
| Conversion deadline | `30s` |

Multiplication used for pixel limits must be overflow-safe. A canceled request
must stop remote staging immediately. If a libvips operation cannot be
interrupted at the C boundary, it remains semaphore-bounded, its result is
discarded after cancellation, and all temporary resources are removed.

## Cache lifecycle

Use these defaults:

| Setting | Default |
| --- | --- |
| Directory | `/var/cache/roaminal/filesystem-image-previews` |
| Target size | `128 MiB` |
| Entry maximum age | `24h` |
| Expiration sweep | `10m` |
| Directory mode | `0700` |
| Cache and staging file mode | `0600` |

The managed directory contains a Roaminal ownership marker, an `fcache-data/`
subdirectory, and a `staging/` subdirectory. If a non-empty configured directory
has no valid marker, disable derivative generation rather than adopting or
deleting its contents. A corrupt fcache index may be recreated only by replacing
the marked `fcache-data/` child; no sibling path is in scope.

At backend startup, build the `fcache` index and call its public `Clear(false)`
operation before accepting requests. Remove orphaned staging files only from a
Roaminal-owned staging subdirectory after validating its ownership marker and
that neither the cache path nor staging path is a symlink. Never recursively
delete the configured cache directory. The cache is therefore ephemeral across
backend processes even when the volume survives a container restart.

Track the key and expiration time of every entry committed by the current
process. Every ten minutes, delete expired tracked keys through the public
`fcache.Delete` API and remove them from the tracker. Stop the janitor with the
backend lifecycle context. A key already removed by size eviction is a harmless
no-op. Capacity-based approximate LRU eviction remains entirely owned by
`fcache` and runs after every completed write.

On a cache read error or corrupt entry, delete that key and attempt one
regeneration. Do not loop. Cache initialization, reads, writes, eviction, or
cleanup failures must never prevent original viewing or download.

## Configuration and Helm

Add canonical backend configuration, file keys, CLI flags, environment
variables, validation, tests, and `docs/configuration.md` entries for:

| Configuration field | Environment variable | Default |
| --- | --- | --- |
| `filesystemImagePreviewCacheDir` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_DIR` | path above |
| `filesystemImagePreviewCacheTargetMiB` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_TARGET_MIB` | `128` |
| `filesystemImagePreviewCacheMaxAge` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_MAX_AGE` | `24h` |
| `filesystemImagePreviewCacheCleanupInterval` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_CLEANUP_INTERVAL` | `10m` |
| `filesystemImagePreviewMaxConversions` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_CONVERSIONS` | `1` |
| `filesystemImagePreviewMaxSourceMiB` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_SOURCE_MIB` | `32` |
| `filesystemImagePreviewMaxOutputMiB` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_OUTPUT_MIB` | `16` |
| `filesystemImagePreviewMaxStaticPixels` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_STATIC_PIXELS` | `100000000` |
| `filesystemImagePreviewMaxFrames` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_FRAMES` | `200` |
| `filesystemImagePreviewMaxAnimatedPixels` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_ANIMATED_PIXELS` | `200000000` |
| `filesystemImagePreviewConversionTimeout` | `ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CONVERSION_TIMEOUT` | `30s` |

The cache directory must be absolute, non-root, not equal to the home, state,
workspace, SSH, or `/tmp` directory, and must not traverse a symlink. All integer
limits must be positive and bounded against integer overflow. Maximum age and
cleanup interval must be at least one minute; the conversion deadline must fit
below the HTTP write timeout. WebP quality, effort, format, and pipeline version
are implementation constants, not deployment settings.

Add one Chart section, `filesystemImagePreview`, containing the matching
operational limits plus:

- `cache.emptyDir.medium`, default empty;
- `cache.emptyDir.sizeLimit`, default `192Mi`;
- `cache.mountPath`, fixed by the template to the backend cache directory and
  not user-overridable.

Render a separate `emptyDir` and mount it only at the image-preview cache path.
Do not reuse `/tmp`, the unified state PVC, or the SSH volume. Add all values to
`values.schema.json`, Chart README, ConfigMap, Deployment environment, and Helm
template tests.

The minimum recommended volume size is:

`target cache + max conversions * (max source + max output) + 16 MiB`

The defaults produce `128 + 1 * (32 + 16) + 16 = 192 MiB`. Document that an
operator increasing concurrency or byte limits must increase `sizeLimit` using
this formula. Kubernetes volume exhaustion must degrade preview generation only.

## Container and local build

The backend builder must install a C compiler, `pkg-config`, and libvips
development headers, then build the backend with CGO enabled. Do not change the
standalone Codex hook builds, which remain static and cross-compiled separately.

Install the compatible libvips runtime package in the final Debian Bookworm
image. Keep dynamic libraries but do not retain compiler tools, headers, package
indexes, or libvips CLI utilities unless required at runtime. The runtime
container remains non-root with a read-only root filesystem; the dedicated
cache mount is its only new writable location.

Update local development and container build prerequisites. Verify the final
binary resolves all libvips shared libraries and that the production image can
generate WebP under UID/GID `1000`.

## Frontend interaction

Replace `readContent`'s positional `download` boolean with an options object that
can express `variant`, `download`, `range`, and `signal` without ambiguous call
sites. Existing non-image callers retain their current behavior.

For an image file:

1. Request `variant=preview` after metadata is loaded.
2. If preview generation fails, request `variant=original` once and preserve the
   current original-image behavior. Do not retry in a loop.
3. Display a dedicated Lucide `ScanSearch` icon button in the preview header,
   adjacent to Download. Its tooltip and accessible name are `View original`.
   Do not use a fullscreen icon and do not show persistent button text.
4. The image itself is not an original-load trigger.
5. When the user activates View original, fetch `variant=original` while keeping
   the compressed preview visible. Show progress in the icon button only.
6. Replace the object URL only after the complete original loads successfully.
   On failure, retain the preview and show the existing concise error toast.
7. Once original content is displayed, leave the button in a disabled/selected
   state with the accessible label `Original loaded`.
8. Download continues to use `download=1`; it never reuses the preview or an
   already loaded object URL.

Reset variant/loading state when the selected file identity changes, abort stale
requests, and revoke every replaced or unmounted object URL. Re-run the existing
fit-to-viewport calculation when either source loads. Preserve the preview's
scroll state and do not introduce scrollbars at the default fit scale.

## Failure handling and observability

Use structured, path-safe logs. Do not log absolute/relative file paths, image
contents, credentials, or terminal data. Connection instance ID, request ID,
cache identity prefix, MIME type, dimensions, frame count, byte counts, duration,
cache hit/miss, and a bounded reason code are allowed.

Log once at INFO when the service starts, including effective limits and libvips
version. Log successful cache-miss conversion summaries at INFO because they
are expensive operations. Cache hits are DEBUG. Log initialization, conversion,
cache corruption, capacity eviction errors, and janitor failures with stable
event names and an error type; do not emit one repeated error per polling tick.

Required failure behavior:

- Unsupported, oversized, malformed, or unpreservable images fall back to the
  original in the frontend.
- A failed explicit original load leaves the derivative visible.
- A cache-disk-full or permission error removes partial files and falls back to
  the original.
- A root revision change uses the existing root recovery flow; stale output is
  never committed under the old token.
- A disconnected connection instance reports the existing FileSystem transport
  error. The cache must not serve a hit without first validating the current
  authorized root and source consistency token.

## Security requirements

- Keep all current authentication, connection-instance authorization, resolved
  root, path traversal, symlink, and source consistency checks ahead of cache
  access.
- Treat source MIME as a preliminary allowlist only; libvips must successfully
  identify and decode the bytes. Do not invoke shell image tools.
- Exclude SVG from derivative generation to avoid remote-resource and script
  semantics. Existing original handling remains unchanged.
- Enforce source bytes, pixel products, frames, output bytes, concurrency, and
  deadline before cache commit. Fail closed on overflow.
- Strip source metadata after applying orientation and color conversion. Cache
  files and directories remain private and are never exposed as static files.
- Cache startup cleanup uses only `fcache.Clear` and a verified Roaminal-owned
  staging directory; it never recursively removes an arbitrary configured path.
- Keep libvips current through pinned dependency/container updates and include
  malicious/truncated-image fixtures in backend tests.

## Test plan and acceptance criteria

### Backend unit and integration tests

- Every derivative identity field independently changes the key and ETag;
  preview and original ETags differ.
- Transparent PNG, JPEG with EXIF orientation, color-profiled PNG/JPEG, static
  WebP, and animated GIF/WebP fixtures produce WebP with no geometric resampling,
  correct visible orientation, alpha, and supported frame timing/loop behavior.
- Unsupported SVG, malformed input, excessive source bytes, dimensions, frames,
  total pixels, output bytes, and conversion deadline return the defined error
  without a cache entry or leftover staging file.
- Concurrent requests for one key execute one remote read and one conversion;
  different misses obey the conversion limit.
- Cache hit streams the stored file without contacting the converter. Corruption
  deletes and regenerates once. TTL cleanup physically deletes expired tracked
  entries, while a tiny target size exercises fcache capacity eviction.
- Startup clears prior cache/staging contents; shutdown stops the janitor and
  libvips lifecycle cleanly.
- API tests cover omitted, preview, original, download precedence, invalid
  variant, range, `If-None-Match`, response MIME/disposition, and root/source
  changes. Non-image content tests remain unchanged.
- A disabled or failed image-preview service does not fail backend startup,
  `/healthz`, original content, or downloads.

### Frontend tests

- Initial image load requests preview, and preview failure falls back to original
  exactly once.
- Clicking the image does not request the original. Activating `ScanSearch` does.
- The derivative remains rendered while the original is pending or fails; a
  successful original atomically replaces it.
- Download always requests `download=1`, independent of the displayed variant.
- File changes and unmounts abort stale requests and revoke object URLs.
- Fit-to-viewport, orientation-aware dimensions, no-default-scrollbar behavior,
  and preview scroll restoration remain covered.

### Container and Helm tests

- The container builds with CGO, contains only runtime libvips dependencies, and
  starts and converts an image as UID/GID `1000` with a read-only root filesystem.
- `ldd` has no unresolved library and a runtime smoke test verifies WebP output.
- Helm defaults render a distinct `192Mi` cache `emptyDir`, the exact cache mount,
  all environment values, and no change to the existing `/tmp` SSH volume.
- Schema tests reject invalid limits and unknown keys. Custom values render
  deterministically.

### Browser regression specification

Update `tests/playwright/workspace/13-filesystem.md` rather than creating another
suite. Its fixture must include a large transparent PNG and an animated image.
The case must verify:

- initial display receives `image/webp`, preserves dimensions/aspect ratio, fits
  without default scrollbars, and does not request/download the original;
- View original is icon-only, has the correct tooltip/accessibility name, does
  not trigger by image click, keeps preview visible while pending, and replaces
  it after success;
- downloaded bytes and MIME match the remote original, not the WebP derivative;
- reopening the unchanged image yields a cache hit with identical preview ETag;
- modifying the remote image changes the preview ETag and visible derivative;
- preview-generation failure still displays the original and produces no
  uncaught page error, console warning/error, or unexpected failed request;
- phone and desktop layouts keep the action reachable and the fitted image
  usable.

The implementation is complete only when focused Go/frontend tests, container
smoke tests, Helm lint/template/schema checks, and the updated Playwright case
pass, permanent docs reflect the new configuration and runtime dependency, and
no original-image, cache, or staging artifacts remain after test cleanup.
