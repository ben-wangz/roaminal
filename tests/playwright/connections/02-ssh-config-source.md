# PW-CONN-002: SSH-config source capabilities and unsupported syntax

Priority: P1. Capabilities: isolated mutable/read-only SSH fixtures. Viewport:
desktop. Run serially.

## Variants

Execute each available source variant against a dedicated release:

1. Writable regular `~/.ssh/config`: source band reports readable/writable and
   Host/Edit/Duplicate/Delete actions are enabled.
2. Directly mounted read-only config: definitions remain readable and Start is
   enabled, while mutations are disabled and the source band explains
   read-only status.
3. Missing config: Local remains usable, the SSH list is empty, and creating a
   Host is available only if the parent SSH directory can safely create it.
4. Unreadable, non-regular, or unsafe source: the manager fails closed, explains
   the capability, and does not overwrite or chmod the source.
5. Config containing `Include`, `Match`, wildcard/multi-alias Host blocks,
   unmanaged IdentityFile paths, and other unsupported directives: only
   concrete supported Host blocks appear. Warnings and advanced-directive
   counts are visible; `Include` content is completely ignored.

## Preservation assertion

For a writable fixture, edit one supported field in a concrete Host block and
verify comments, newline style, blank lines, unsupported directives, Match
blocks, and excluded Host blocks are byte-for-byte preserved outside the
intended edit. The UI must expose neither a raw editor nor a raw viewer.

## Pass gate

Run the global diagnostics gate. Source warnings are product data and must not
appear as browser console warnings. Retain pre/post fixture hashes and restore
the original config.
