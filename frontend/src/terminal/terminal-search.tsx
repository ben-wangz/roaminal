import { useState } from 'react';
import type { TerminalRuntime } from './terminal-runtime';

export function TerminalSearch({ runtime, onClose }: { runtime: TerminalRuntime; onClose: () => void }) {
  const [query, setQuery] = useState('');
  const [regex, setRegex] = useState(false);
  const [whole, setWhole] = useState(false);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const find = (forward: boolean) => {
    if (!query) return;
    if (forward) runtime.find(query, { regex, wholeWord: whole, caseSensitive });
    else runtime.findPrevious(query, { regex, wholeWord: whole, caseSensitive });
  };
  return (
    <div className="search-bar">
      <input
        autoFocus
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') find(!event.shiftKey);
          if (event.key === 'Escape') onClose();
        }}
        placeholder="Search terminal"
      />
      <label>
        <input type="checkbox" checked={caseSensitive} onChange={(event) => setCaseSensitive(event.target.checked)} />{' '}
        Aa
      </label>
      <label>
        <input type="checkbox" checked={whole} onChange={(event) => setWhole(event.target.checked)} /> Ab
      </label>
      <label>
        <input type="checkbox" checked={regex} onChange={(event) => setRegex(event.target.checked)} /> .*{' '}
      </label>
      <button onClick={() => find(false)} aria-label="Previous result">
        ↑
      </button>
      <button onClick={() => find(true)} aria-label="Next result">
        ↓
      </button>
      <button onClick={onClose} aria-label="Close search">
        ×
      </button>
    </div>
  );
}
