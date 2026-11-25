import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import MagicString from 'magic-string';
import { normalizePath, type Plugin } from 'vite';

const xtermPackage = '@xterm/xterm';
const pinnedVersion = '6.1.0-beta.197';
const resolve = createRequire(import.meta.url).resolve;
const bridge = normalizePath(fileURLToPath(new URL('./src/terminal/xterm-composition-commit.ts', import.meta.url)));

// Patch the deferred read in the pinned browser bundle and let xterm's keydown
// path settle that same read before sending a key. xterm still extracts and
// sends the range and handles candidate display, without modifying npm files.
const delayedCommit = [
  'this._isSendingComposition=!0,setTimeout(()=>{if(this._isSendingComposition){',
  'this._isSendingComposition=!1;let r;if(e.start+=this._dataAlreadySent.length,this._isComposing)',
  'r=this._textarea.value.substring(e.start,this._compositionPosition.start);',
  'else{let s=this._textarea.value,o=i.length>0&&s.endsWith(i)?s.length-i.length:s.length;',
  'r=s.substring(e.start,Math.max(e.start,o))}r.length>0&&this._coreService.triggerDataEvent(r,!0)}},0)',
].join('');
const boundedCommit = delayedCommit
  .replace('setTimeout(()=>', '__roaminalDeferXtermComposition(this._textarea, value=>')
  .replaceAll('this._textarea.value', 'value')
  .replace('}},0)', '}})');
const keydownCommit = 'this._finalizeComposition(!1)}return t.keyCode===229';
const boundedKeydown = `__roaminalFlushXtermComposition(this._textarea)||${keydownCommit}`;

export function xtermImePatch(): Plugin {
  const { version } = JSON.parse(readFileSync(resolve(`${xtermPackage}/package.json`), 'utf8'));
  if (version !== pinnedVersion) throw new Error(`Review Roaminal's IME patch for ${xtermPackage} ${version}`);
  const entry = normalizePath(resolve(`${xtermPackage}/lib/xterm.mjs`));
  return {
    name: 'roaminal-xterm-ime',
    enforce: 'pre',
    config: () => ({
      resolve: { alias: [{ find: /^@xterm\/xterm$/, replacement: entry }] },
      // The dev optimizer would otherwise prebundle xterm without the patch.
      optimizeDeps: { exclude: [xtermPackage] },
    }),
    transform: {
      filter: { id: /\/@xterm\/xterm\/lib\/xterm\.mjs(?:\?|$)/ },
      handler(code, id) {
        if (id.split('?', 1)[0] !== entry) return;
        const patch = new MagicString(code);
        const replace = (from: string, to: string) => {
          const start = code.indexOf(from);
          if (start < 0 || code.indexOf(from, start + 1) !== -1) {
            this.error(`Cannot apply Roaminal's IME patch to ${xtermPackage} ${pinnedVersion}`);
          }
          patch.overwrite(start, start + from.length, to);
        };
        replace(delayedCommit, boundedCommit);
        replace(keydownCommit, boundedKeydown);
        patch.prepend(`import { deferXtermComposition as __roaminalDeferXtermComposition, flushXtermComposition as __roaminalFlushXtermComposition } from ${JSON.stringify(bridge)};\n`);
        return { code: patch.toString(), map: patch.generateMap({ hires: 'boundary' }) };
      },
    },
  };
}
