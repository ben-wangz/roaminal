# PW-WS-013: FileSystem tree, preview, and upload

Priority: P1. Capabilities: one live SSH connection instance whose tmux session
contains a disposable fixture directory with Markdown, text, image, video, PDF,
hidden, symlink, and nested-directory entries. Viewports: desktop and phone
portrait. Use the existing diagnostics listeners and never print file contents
or credentials.

## Procedure and assertions

1. Open the fixture SSH connection instance and click its folder extension. The
   main workspace body is replaced by FileSystem; no Terminal/FileSystem tab
   bar is rendered, and no new window, browser fullscreen request, command
   action, or terminal action is created. The root shows `Active pane` when
   tmux probing succeeds.
2. Verify the root directory loads only its first level, directories appear
   before files, hidden files are visible by default, and the fallback status is
   explicit when tmux probing is unavailable. Toggle hidden files and verify only
   the tree visibility changes.
3. Single-click a file. Selection changes but no stat/content request or preview
   change occurs. Double-click Markdown and text files and verify the preview
   pane opens with the correct viewer, source/rendered Markdown switching is
   safe, and file content is never interpreted as executable HTML or a command.
4. Double-click a directory to enter it. Verify lazy loading, expand/collapse,
   breadcrumb navigation, keyboard Enter activation, and a directory refresh.
   Confirm that a broken symlink is listed but cannot be read as file content.
5. Resize the tree/preview divider with pointer and arrow-key input on desktop.
   Verify it stays within the documented bounds. On phone portrait, verify the
   page switches between tree and preview views inside the page and does not
   call either Fullscreen API.
6. Right-click the root, a directory, and a file. The menu contains only copy
   absolute path, copy root-relative path, refresh, and for directories the one
   unified upload action. Clipboard values contain plain paths without quotes,
   shell prefixes, or commands. Escape and outside click close the menu.
7. Use the unified upload action, choose local files and a local directory, and
   verify the confirmation dialog shows target relative/absolute paths, counts,
   names, total size, default refuse policy, and automatic transport selection.
   Before confirmation, assert that no upload request or remote write occurs.
8. Cancel once and confirm that no upload job exists. Repeat and confirm the
   upload; verify a 202 upload ID, non-blocking progress, observable `rsync` or
   `scp` transport, cancel behavior, partial-failure paths, and refresh of only
   the target directory after completion.
9. Use the sidebar Agent control to return to Terminal, then the folder control
   to enter FileSystem again. Repeat between two connection instances. Verify
   only the selected mode occupies the main workspace body, each instance keeps
   its own root, expanded paths, selection, preview, and upload status, and no
   file data or root path from one instance appears in the other.
10. Move the active tmux pane to another directory and trigger a tree/content
    request with the old root revision. Verify the UI clears the old tree,
    reloads the returned root, and shows a concise root-changed notice. Make a
    failed tmux probe and verify a later request retries after the short failure
    cache rather than remaining permanently unavailable.

## Pass gate

All expected protected API responses are correlated with the action that caused
them. Fail on unexpected console/page errors, a request containing an absolute
client-supplied operation path or arbitrary command, leaked file content in
diagnostics, duplicate upload jobs, stale cross-instance state, or any full
screen/new-window behavior.
