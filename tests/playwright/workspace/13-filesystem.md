# PW-WORK-013: FileSystem tree, preview, and upload

Priority: P1. Capabilities: one live SSH connection instance whose tmux session
contains a disposable fixture directory with Markdown, text, a large transparent
PNG, an animated image, video, PDF, hidden, symlink, and nested-directory
entries. Viewports: desktop and phone portrait. Use the existing diagnostics
listeners and never print file contents or credentials.

## Procedure and assertions

1. Open the fixture SSH connection instance and click the Files rail icon (or
   the connection card's Files action). The left tool surface changes to
   the active instance's file tree while the right Terminal remains visible;
   no Terminal/FileSystem tab bar, new window, automatic browser fullscreen
   request, command action, or terminal action is created. The root shows
   `Active pane` when tmux probing succeeds.
2. Verify the root directory loads only its first level, directories appear
   before files, hidden files are visible by default, and the fallback status is
   explicit when tmux probing is unavailable. Toggle hidden files and verify only
   the tree visibility changes.
3. Single-click a file. Selection changes but no stat/content request or right
   content change occurs. Double-click Markdown and text files and verify the
   right Terminal content is replaced by the correct File preview viewer;
   source/rendered Markdown switching is safe, the icon-only `Back to
   Terminal` control is present, and file content is never interpreted as
   executable HTML or a command. The left tree remains visible and unchanged.
   For the PNG and animated image, record the content responses and assert that
   the initial request uses `variant=preview`, returns `Content-Type: image/webp`,
   and does not request `variant=original`, `download=1`, or the complete source.
   The displayed image keeps the source dimensions/aspect ratio and fits the
   preview body without a default horizontal or vertical scrollbar. Clicking
   the image itself does not load the original.
4. Double-click a directory to enter it. Verify lazy loading, expand/collapse,
   breadcrumb navigation, keyboard Enter activation, and the directory context
   menu's Refresh action. The Root row has no duplicate refresh button. A
   reopened directory shows cached children immediately and then refreshes in
   the background; a failed refresh keeps the stale children visible. If a
   directory was removed remotely, its cached descendants, expansion state,
   selection, and preview are pruned. Confirm that a broken symlink is listed
   but cannot be read as file content.
5. On desktop, verify the Files tool occupies the same bounded left tool
   surface as Connections and Virtual keyboard and the right preview fills the
   primary work area. The old in-page tree/preview divider is not rendered.
   On phone portrait, verify Files opens as the shared left drawer and the
   preview uses the primary work area without calling either Fullscreen API.
   The explicit topbar fullscreen control remains independent and is not part
   of Files navigation.
6. Right-click the root, a directory, and a file. The menu contains only copy
   absolute path, copy root-relative path, refresh, and for directories the one
   unified upload action. Clipboard values contain plain paths without quotes,
   shell prefixes, or commands. On phone/tablet, every row including Root has a
   visible `More actions` button with the same menu, and a 550ms long press with
   less than 10px movement opens that same menu without opening a preview,
   toggling a directory, scrolling cancellation, or a duplicate browser menu.
   The Context Menu key and Shift+F10 open the same menu from a focused row.
   Escape and outside pointer down close the menu.
7. Use the unified upload action, choose local files and a local directory, and
   verify the confirmation dialog shows target relative/absolute paths, counts,
   names, total size, default refuse policy, and automatic transport selection.
   Before confirmation, assert that no upload request or remote write occurs.
8. Cancel once and confirm that no upload job exists. Repeat and confirm the
   upload; verify a 202 upload ID, non-blocking progress, observable `rsync` or
   `scp` transport, cancel behavior, partial-failure paths, and refresh of only
   the target directory after completion. For an image whose preview is shown,
   activate the icon-only `View original` control and assert its tooltip and
   accessible name, that the WebP remains visible while the original request is
   pending, and that the source image replaces it only after a successful
   `variant=original` response. Download the same image and assert that the
   request uses `download=1` and the received bytes/MIME are the remote original,
   not the WebP derivative.
9. Click the single Refresh control in the `FILES` heading. Verify it
   re-probes the tmux PWD, refreshes the root and every currently expanded
   directory with at most three concurrent listing requests, preserves
   expansion/selection/preview when the root revision is unchanged, and clears
   the tree with a root-changed notice when the revision changes. With a long
   Markdown file open, scroll both rendered Markdown and Markdown source to a
   non-zero position before the refresh; the preview remains open and restores
   both scroll offsets after the tree and content requests finish. Repeat with
   one automatic refresh and with a deliberately changed active-pane root: a
   transient root recovery must not discard the open preview or reset its
   reading position. The refresh request uses `cache: no-store` and successful
   root/entries responses carry `Cache-Control: no-store`. Reopen the unchanged
   image and assert that the preview response has the same ETag and is served
   without another source transfer. Modify the remote image, refresh the tree,
   reopen it, and assert that the preview ETag and visible derivative change.
   Force one preview-generation failure and assert that the frontend falls back
   to the original exactly once without an uncaught page error, console
   warning/error, or unexpected failed request. Repeat the image checks on
   desktop and phone portrait, keeping the preview actions reachable.
10. Open the auto-refresh menu beside the global Refresh control. Verify the
    browser-wide preference offers Off, 30 seconds, 60 seconds, 2 minutes, and
    5 minutes, defaults to 60 seconds, persists after reload, pauses while the
    Files tool is inactive or the document is hidden, and performs one overdue
    refresh on visibility resume without overlapping a manual refresh.
    Directory refreshes and preview reads do not reset the interval.
11. With the Files tool selected, activate a file and verify the right side
   changes to File preview while the tree remains mounted. Click the icon-only
   `Back to Terminal` control and verify only the right side returns to
   Terminal; the Files tool, root, expanded paths, selection, and tree scroll
   remain unchanged. The Agent robot opens Agent details and never changes
   the right content accidentally. Repeat between two connection instances:
   selecting another instance clears the stale preview target, binds one tree
   to the new instance, and never shows cross-instance file data.
12. Move the active tmux pane to another directory and trigger a tree/content
    request with the old root revision. Verify the UI clears the old tree,
    reloads the returned root, and shows a concise root-changed notice. Make a
    failed tmux probe and verify a later request retries after the short failure
    cache rather than remaining permanently unavailable. Inject one transient
    `filesystem_transport_unavailable`/retryable remote-monitor response while
    keeping the connection instance live; verify bounded automatic retry
    recovers both features without remounting or recreating the instance. A
    non-retryable `filesystem_no_transport` response must keep Terminal usable,
    suppress endless retry, and explain that a fresh SSH connection instance is
    required.

## Pass gate

All expected protected API responses are correlated with the action that caused
them. Fail on unexpected console/page errors, a request containing an absolute
client-supplied operation path or arbitrary command, leaked file content in
diagnostics, duplicate upload jobs, stale cross-instance state, or any
automatic fullscreen/new-window behavior. An explicit user activation of the
independent fullscreen control is allowed only in its dedicated regression
case. The browser may report one `net::ERR_ABORTED` for
a `blob:` document while an image/video/PDF preview is being closed; treat only
that document-teardown case as expected, and fail all other aborted requests.
