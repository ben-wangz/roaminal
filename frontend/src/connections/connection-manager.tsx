import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ExternalLink, Plus, RefreshCw, Settings } from 'lucide-react';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import {
  createDefinition,
  deleteDefinition,
  deleteKey,
  duplicateDefinition,
  generateKey,
  loadDefinitions,
  loadKeys,
  updateDefinition,
  type ConnectionDefinition,
  type DefinitionCollection,
  type GenerationRequest,
  type SSHKey,
} from './connection-api';
import { ConnectionDefinitionEditor } from './connection-definition-editor';
import {
  bodyFrom,
  draftFrom,
  emptyDraft,
  type ConnectionDraft,
  type ConnectionEditor,
} from './connection-definition-model';
import { filterDefinitions, preserveLastOptions } from './connection-manager-state';
import { ConnectionDefinitionRow, LocalConnectionRow, SourceBand } from './connection-manager-rows';
import { SSHKeyGenerationDialog } from './ssh-key-generation-dialog';
import { SSHKeysPanel } from './ssh-keys-panel';
import type { ToastKind } from '../ui/toast';

type Props = {
  connections: ConnectionInstanceSummary[];
  onConnect: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<void>;
  onGenerated: (instance: ConnectionInstanceSummary) => Promise<void>;
  onOpenWorkspace: () => void;
  onOpenAppearance: () => void;
  onToast: (message: string, kind?: ToastKind) => void;
};

export function ConnectionManager({ connections, onConnect, onGenerated, onOpenWorkspace, onOpenAppearance, onToast }: Props) {
  const [tab, setTab] = useState<'connections' | 'keys'>('connections');
  const [definitions, setDefinitions] = useState<DefinitionCollection | null>(null);
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [etag, setETag] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<ConnectionEditor>(null);
  const [draft, setDraft] = useState<ConnectionDraft>(emptyDraft);
  const [generation, setGeneration] = useState<GenerationRequest | null>(null);
  const onToastRef = useRef(onToast);
  onToastRef.current = onToast;

  const refreshSources = useCallback(async () => {
    setBusy(true);
    try {
      const [definitionResult, keyResult] = await Promise.all([loadDefinitions(), loadKeys()]);
      setDefinitions((previous) => preserveLastOptions(previous, definitionResult.data));
      setETag(definitionResult.etag);
      setKeys(keyResult.keys);
    } catch (error) {
      onToastRef.current((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void refreshSources();
  }, [refreshSources]);

  const visible = useMemo(() => filterDefinitions(definitions, query), [definitions, query]);

  const optionsAvailable = !definitions?.tmuxOptionsSource || definitions.tmuxOptionsSource.status === 'missing' || definitions.tmuxOptionsSource.status === 'available';

  function beginEditor(mode: 'create' | 'edit', definition?: ConnectionDefinition) {
    setEditor({ mode, definition });
    setDraft(draftFrom(definition, keys));
  }

  function beginGeneration(algorithm: GenerationRequest['algorithm']) {
    const existing = keys.find((key) => key.algorithm === algorithm);
    if (existing) {
      onToast(
        `${algorithm === 'ed25519' ? 'Ed25519' : 'RSA'} key already exists (${existing.fileName}). Delete it before generating another.`,
        'error',
      );
      return;
    }
    setGeneration({
      algorithm,
      rsaBits: algorithm === 'rsa' ? 3072 : null,
      fileName: algorithm === 'rsa' ? 'id_rsa' : 'id_ed25519',
      comment: '',
    });
  }

  async function saveDefinition(event: React.FormEvent) {
    event.preventDefault();
    if (!etag) {
      onToast('Config ETag unavailable; refresh first.', 'error');
      return;
    }
    setBusy(true);
    try {
      const result =
        editor?.mode === 'edit' && editor.definition
          ? await updateDefinition(editor.definition.connectionDefinitionId, bodyFrom(draft), etag)
          : await createDefinition(bodyFrom(draft), etag);
      setDefinitions(result.data);
      setETag(result.etag);
      setEditor(null);
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  async function copyDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias) return;
    const alias = window.prompt('New host alias', `${definition.hostAlias}-copy`);
    if (!alias) return;
    setBusy(true);
    try {
      const result = await duplicateDefinition(definition.connectionDefinitionId, alias.trim(), etag);
      setDefinitions(result.data);
      setETag(result.etag);
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  async function removeDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias || !window.confirm(`Delete Host ${definition.hostAlias}?`)) return;
    setBusy(true);
    try {
      const result = await deleteDefinition(definition.connectionDefinitionId, etag);
      setDefinitions(result.data);
      setETag(result.etag);
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  async function startGeneration(event: React.FormEvent) {
    event.preventDefault();
    if (!generation) return;
    setBusy(true);
    try {
      const instance = await generateKey(generation);
      setGeneration(null);
      await onGenerated(instance);
      onToast(`Key generation connection ${instance.connectionInstanceId.slice(0, 8)} is ready.`, 'success');
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  async function removeKey(key: SSHKey) {
    if (key.readOnly || !window.confirm(`Delete SSH key ${key.fileName} and its public key?`)) return;
    setBusy(true);
    try {
      await deleteKey(key.keyId);
      setKeys((current) => current.filter((item) => item.keyId !== key.keyId));
      onToast(`Deleted ${key.fileName}.`, 'success');
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="connection-manager" aria-label="Connection manager">
      <header className="manager-header">
        <div>
          <p className="eyebrow">ROAMINAL</p>
          <h1>Connections</h1>
        </div>
        <div className="manager-header-actions">
          <button
            className="icon-button"
            type="button"
            onClick={() => void refreshSources()}
            disabled={busy}
            aria-label="Refresh SSH sources"
            title="Refresh SSH sources"
          >
            <RefreshCw size={17} aria-hidden="true" className={busy ? 'spin' : ''} />
          </button>
          <button className="text-button" type="button" onClick={onOpenWorkspace}>
            <ExternalLink size={15} aria-hidden="true" /> Workspace
          </button>
          <button className="text-button" type="button" onClick={onOpenAppearance}>
            <Settings size={15} aria-hidden="true" /> Appearance
          </button>
        </div>
      </header>
      <nav className="manager-tabs" aria-label="Connection manager sections">
        <button type="button" className={tab === 'connections' ? 'active' : ''} onClick={() => setTab('connections')}>
          Connections
        </button>
        <button type="button" className={tab === 'keys' ? 'active' : ''} onClick={() => setTab('keys')}>
          Keys <span>{keys.length}</span>
        </button>
      </nav>
      {tab === 'connections' ? (
        <>
          {definitions && <SourceBand source={definitions.configSource} label="SSH config" />}
          {definitions?.tmuxOptionsSource && definitions.tmuxOptionsSource.status !== 'missing' && (
            <SourceBand source={definitions.tmuxOptionsSource} label="Roaminal tmux options" />
          )}
          <div className="manager-toolbar">
            <label className="manager-search">
              <span>Filter</span>
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Alias or destination"
                aria-label="Filter connections"
              />
            </label>
            <button
              className="primary"
              type="button"
              onClick={() => beginEditor('create')}
              disabled={!definitions?.configSource.writable || busy}
            >
              <Plus size={15} aria-hidden="true" /> Host
            </button>
          </div>
          <div className="connection-list">
            <LocalConnectionRow onConnect={() => void onConnect('local')} />
            {visible
              .filter((definition) => definition.type === 'ssh')
              .map((definition) => (
                <ConnectionDefinitionRow
                  key={definition.connectionDefinitionId}
                  definition={definition}
                  editable={Boolean(definitions?.configSource.writable)}
                  connections={connections}
                  onConnect={onConnect}
                  onEdit={() => beginEditor('edit', definition)}
                  onDuplicate={() => void copyDefinition(definition)}
                  onDelete={() => void removeDefinition(definition)}
                />
              ))}
            {!visible.some((definition) => definition.type === 'ssh') && (
              <div className="manager-empty">No matching SSH definitions.</div>
            )}
          </div>
        </>
      ) : (
        <SSHKeysPanel
          keys={keys}
          connections={connections}
          onGenerate={beginGeneration}
          onDelete={removeKey}
          onToast={onToast}
        />
      )}
      {editor && (
        <ConnectionDefinitionEditor
          editor={editor}
          draft={draft}
          keys={keys}
          busy={busy}
          optionsAvailable={optionsAvailable}
          onDraft={setDraft}
          onSave={(event) => void saveDefinition(event)}
          onClose={() => setEditor(null)}
        />
      )}
      {generation && (
        <SSHKeyGenerationDialog
          value={generation}
          existingAlgorithms={new Set(keys.map((key) => key.algorithm))}
          busy={busy}
          onChange={setGeneration}
          onSubmit={(event) => void startGeneration(event)}
          onClose={() => setGeneration(null)}
        />
      )}
    </section>
  );
}
