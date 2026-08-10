import { useEffect, useMemo, useState } from 'react';
import { Clipboard, Copy, Edit3, ExternalLink, Home, KeyRound, Play, Plus, RefreshCw, ShieldAlert, Trash2, X } from 'lucide-react';
import { Modal } from '../ui/modal';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import { createDefinition, deleteDefinition, deleteKey, duplicateDefinition, generateKey, loadDefinitions, loadKeys, publicKey, updateDefinition, type ConnectionDefinition, type ConfigSource, type DefinitionCollection, type GenerationRequest, type SSHKey } from './connection-api';

type Props = {
  instances: ConnectionInstanceSummary[];
	onConnect: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<void>;
  onGenerated: (instance: ConnectionInstanceSummary) => Promise<void>;
  onOpenWorkspace: () => void;
  onToast: (message: string) => void;
};
type Draft = { hostAlias: string; hostName: string; user: string; port: string; identities: string[]; identitiesOnly: string; strictHostKeyChecking: string; userKnownHostsFile: string; serverAliveInterval: string; tmuxEnabled: boolean; tmuxSessionName: string };
type Editor = { mode: 'create' | 'edit'; definition?: ConnectionDefinition } | null;

const emptyDraft: Draft = { hostAlias: '', hostName: '', user: 'root', port: '22', identities: [], identitiesOnly: '', strictHostKeyChecking: '', userKnownHostsFile: '', serverAliveInterval: '15', tmuxEnabled: false, tmuxSessionName: '' };

function draftFrom(definition?: ConnectionDefinition, keys: SSHKey[] = []): Draft {
  if (!definition) {
    const identities = keys.filter((key) => key.algorithm === 'ed25519' && key.status === 'available').slice(0, 1).map((key) => key.fileName);
    return { ...emptyDraft, identities, identitiesOnly: identities.length ? 'yes' : '' };
  }
  return { hostAlias: definition.hostAlias || '', hostName: definition.hostName || '', user: definition.user || '', port: definition.port ? String(definition.port) : '', identities: [...definition.identityFileNames], identitiesOnly: definition.identitiesOnly || '', strictHostKeyChecking: definition.strictHostKeyChecking || '', userKnownHostsFile: definition.userKnownHostsFile || '', serverAliveInterval: definition.serverAliveInterval ? String(definition.serverAliveInterval) : '', tmuxEnabled: Boolean(definition.tmux?.enabled), tmuxSessionName: definition.tmux?.sessionName || '' };
}

function bodyFrom(draft: Draft): Record<string, unknown> {
  return { type: 'ssh', hostAlias: draft.hostAlias.trim(), hostName: draft.hostName.trim() || null, user: draft.user.trim() || null, port: draft.port ? Number(draft.port) : null, identityFileNames: draft.identities, identitiesOnly: draft.identitiesOnly || null, strictHostKeyChecking: draft.strictHostKeyChecking || null, userKnownHostsFile: draft.userKnownHostsFile || null, serverAliveInterval: draft.serverAliveInterval ? Number(draft.serverAliveInterval) : null, tmux: draft.tmuxEnabled ? { enabled: true, sessionName: draft.tmuxSessionName } : { enabled: false, sessionName: '' } };
}

export function ConnectionManager({ instances, onConnect, onGenerated, onOpenWorkspace, onToast }: Props) {
  const [tab, setTab] = useState<'connections' | 'keys'>('connections');
  const [definitions, setDefinitions] = useState<DefinitionCollection | null>(null);
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [etag, setETag] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false);
  const [editor, setEditor] = useState<Editor>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [generation, setGeneration] = useState<GenerationRequest | null>(null);

  async function refreshSources() {
    setBusy(true);
    try {
      const [definitionResult, keyResult] = await Promise.all([loadDefinitions(), loadKeys()]);
      setDefinitions(definitionResult.data);
      setETag(definitionResult.etag);
      setKeys(keyResult.keys);
    } catch (error) { onToast((error as Error).message); }
    finally { setBusy(false); }
  }
  useEffect(() => { void refreshSources(); }, []);

  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return (definitions?.definitions || []).filter((definition) => !normalized || `${definition.hostAlias || ''} ${definition.hostName || ''} ${definition.user || ''}`.toLowerCase().includes(normalized));
  }, [definitions, query]);

  function beginEditor(mode: 'create' | 'edit', definition?: ConnectionDefinition) { setEditor({ mode, definition }); setDraft(draftFrom(definition, keys)); }
  function beginGeneration(algorithm: GenerationRequest['algorithm']) {
    const existing = keys.find((key) => key.algorithm === algorithm);
    if (existing) {
      onToast(`${algorithm === 'ed25519' ? 'Ed25519' : 'RSA'} key already exists (${existing.fileName}). Delete it before generating another.`);
      return;
    }
    setGeneration({ algorithm, rsaBits: algorithm === 'rsa' ? 3072 : null, fileName: algorithm === 'rsa' ? 'id_rsa' : 'id_ed25519', comment: '' });
  }
  async function saveDefinition(event: React.FormEvent) {
    event.preventDefault();
    if (!etag) { onToast('Config ETag unavailable; refresh first.'); return; }
    setBusy(true);
    try {
      const result = editor?.mode === 'edit' && editor.definition ? await updateDefinition(editor.definition.connectionDefinitionId, bodyFrom(draft), etag) : await createDefinition(bodyFrom(draft), etag);
      setDefinitions(result.data); setETag(result.etag); setEditor(null);
    } catch (error) { onToast((error as Error).message); }
    finally { setBusy(false); }
  }
  async function copyDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias) return;
    const alias = window.prompt('New host alias', `${definition.hostAlias}-copy`);
    if (!alias) return;
    setBusy(true);
    try { const result = await duplicateDefinition(definition.connectionDefinitionId, alias.trim(), etag); setDefinitions(result.data); setETag(result.etag); }
    catch (error) { onToast((error as Error).message); }
    finally { setBusy(false); }
  }
  async function removeDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias || !window.confirm(`Delete Host ${definition.hostAlias}?`)) return;
    setBusy(true);
    try { const result = await deleteDefinition(definition.connectionDefinitionId, etag); setDefinitions(result.data); setETag(result.etag); }
    catch (error) { onToast((error as Error).message); }
    finally { setBusy(false); }
  }
  async function startGeneration(event: React.FormEvent) {
    event.preventDefault();
    if (!generation) return;
    setBusy(true);
    try { const instance = await generateKey(generation); setGeneration(null); await onGenerated(instance); onToast(`Key generation connection ${instance.id.slice(0, 8)} is ready.`); }
    catch (error) { onToast((error as Error).message); }
    finally { setBusy(false); }
  }

  return <section className="connection-manager" aria-label="Connection manager">
    <header className="manager-header"><div><p className="eyebrow">ROAMINAL</p><h1>Connections</h1></div><div className="manager-header-actions"><button className="icon-button" type="button" onClick={() => void refreshSources()} disabled={busy} aria-label="Refresh SSH sources" title="Refresh SSH sources"><RefreshCw size={17} aria-hidden="true" className={busy ? 'spin' : ''} /></button><button className="text-button" type="button" onClick={onOpenWorkspace}><ExternalLink size={15} aria-hidden="true" /> Workspace</button></div></header>
    <nav className="manager-tabs" aria-label="Connection manager sections"><button type="button" className={tab === 'connections' ? 'active' : ''} onClick={() => setTab('connections')}>Connections</button><button type="button" className={tab === 'keys' ? 'active' : ''} onClick={() => setTab('keys')}>Keys <span>{keys.length}</span></button></nav>
    {tab === 'connections' ? <>
      {definitions && <SourceBand source={definitions.configSource} label="SSH config" />}
      {definitions?.tmuxOptionsSource && definitions.tmuxOptionsSource.status !== 'missing' && <SourceBand source={definitions.tmuxOptionsSource} label="Roaminal tmux options" />}
      <div className="manager-toolbar"><label className="manager-search"><span>Filter</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Alias or destination" aria-label="Filter connections" /></label><button className="primary" type="button" onClick={() => beginEditor('create')} disabled={!definitions?.configSource.writable || busy}><Plus size={15} aria-hidden="true" /> Host</button></div>
      <div className="connection-list">
        <LocalRow onConnect={() => void onConnect('local')} />
        {visible.filter((definition) => definition.type === 'ssh').map((definition) => <DefinitionRow key={definition.connectionDefinitionId} definition={definition} editable={Boolean(definitions?.configSource.writable)} instances={instances} onConnect={onConnect} onEdit={() => beginEditor('edit', definition)} onDuplicate={() => void copyDefinition(definition)} onDelete={() => void removeDefinition(definition)} />)}
        {!visible.some((definition) => definition.type === 'ssh') && <div className="manager-empty">No matching SSH definitions.</div>}
      </div>
    </> : <KeysPanel keys={keys} instances={instances} onGenerate={beginGeneration} onDelete={async (key) => {
      if (key.readOnly || !window.confirm(`Delete SSH key ${key.fileName} and its public key?`)) return;
      setBusy(true);
      try { await deleteKey(key.keyId); setKeys((current) => current.filter((item) => item.keyId !== key.keyId)); onToast(`Deleted ${key.fileName}.`); }
      catch (error) { onToast((error as Error).message); }
      finally { setBusy(false); }
    }} onToast={onToast} />}
    {editor && <DefinitionEditor editor={editor} draft={draft} keys={keys} busy={busy} onDraft={setDraft} onSave={(event) => void saveDefinition(event)} onClose={() => setEditor(null)} />}
    {generation && <GenerationDialog value={generation} existingAlgorithms={new Set(keys.map((key) => key.algorithm))} busy={busy} onChange={setGeneration} onSubmit={(event) => void startGeneration(event)} onClose={() => setGeneration(null)} />}
  </section>;
}

function SourceBand({ source, label = 'SSH config' }: { source: ConfigSource; label?: string }) {
  const warnings = source.warnings || [];
  const blockers = source.blockers || [];
  return <div className={`source-band ${source.writable ? 'writable' : 'readonly'}`}><div><strong>{label}</strong><span>{source.status}</span></div><div className="source-capabilities"><span>{source.readable ? 'read' : 'unreadable'}</span><span>{source.writable ? 'write' : 'read-only'}</span>{warnings.length > 0 && <span>{warnings.length} warnings</span>}</div>{(blockers.length > 0 || source.reason) && <small>{source.reason || blockers.join(', ')}</small>}</div>;
}
function LocalRow({ onConnect }: { onConnect: () => void }) { return <article className="connection-row local-row"><div className="connection-row-main"><span className="row-icon local-icon"><Home size={17} aria-hidden="true" /></span><div><strong>Local</strong><small>/workspace</small></div></div><button className="icon-button play-button" type="button" onClick={onConnect} aria-label="Start local connection" title="Start local connection"><Play size={16} fill="currentColor" aria-hidden="true" /></button></article>; }

function DefinitionRow({ definition, editable, instances, onConnect, onEdit, onDuplicate, onDelete }: { definition: ConnectionDefinition; editable: boolean; instances: ConnectionInstanceSummary[]; onConnect: (id: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<void>; onEdit: () => void; onDuplicate: () => void; onDelete: () => void }) {
  const live = instances.filter((instance) => instance.type === 'ssh' && instance.lifecycle === 'live' && instance.sourceHostAlias === definition.hostAlias);
  const destination = [definition.user, definition.hostName || definition.hostAlias].filter(Boolean).join('@') + (definition.port ? `:${definition.port}` : '');
  return <article className="connection-row"><div className="connection-row-main"><span className="row-icon"><KeyRound size={17} aria-hidden="true" /></span><div className="connection-copy"><strong>{definition.hostAlias}</strong><small>{destination || 'OpenSSH config destination'}</small><small className="row-facts">{definition.identityFileNames.length} managed keys | {definition.hostVerificationAssessment === 'weakened' ? 'weakened trust' : definition.hostVerificationAssessment}{definition.tmux?.enabled ? ` | tmux:${definition.tmux.sessionName}` : ''}</small></div></div><div className="connection-row-status">{definition.warnings.length + definition.advancedDirectiveCount > 0 && <span className="warning-badge"><ShieldAlert size={13} aria-hidden="true" /> {definition.warnings.length + definition.advancedDirectiveCount}</span>}{definition.tmux?.enabled && <span className="tmux-badge">tmux</span>}{(!editable || definition.capabilities.edit === false) && <span className="readonly-badge">read-only</span>}</div><div className="connection-row-actions"><button className="icon-button" type="button" onClick={() => void onConnect(definition.connectionDefinitionId, undefined, Boolean(definition.tmux?.enabled))} aria-label={`Connect to ${definition.hostAlias}`} title={`Connect to ${definition.hostAlias}`}><Play size={16} fill="currentColor" aria-hidden="true" /></button>{live.map((instance) => <button className="icon-button reuse-button" key={instance.id} type="button" onClick={() => void onConnect(definition.connectionDefinitionId, instance.id, Boolean(definition.tmux?.enabled))} aria-label={`Open over existing ${instance.id.slice(0, 8)} transport`} title="Open over existing transport"><ExternalLink size={15} aria-hidden="true" /></button>)}<button className="icon-button" type="button" onClick={onEdit} disabled={!editable || definition.capabilities.edit === false} aria-label={`Edit ${definition.hostAlias}`} title="Edit connection"><Edit3 size={15} aria-hidden="true" /></button><button className="icon-button" type="button" onClick={onDuplicate} disabled={!editable || definition.capabilities.edit === false} aria-label={`Duplicate ${definition.hostAlias}`} title="Duplicate connection"><Copy size={15} aria-hidden="true" /></button><button className="icon-button danger-icon" type="button" onClick={onDelete} disabled={!editable || definition.capabilities.delete === false} aria-label={`Delete ${definition.hostAlias}`} title="Delete connection"><Trash2 size={15} aria-hidden="true" /></button></div></article>;
}

function KeysPanel({ keys, instances, onGenerate, onDelete, onToast }: { keys: SSHKey[]; instances: ConnectionInstanceSummary[]; onGenerate: (algorithm: GenerationRequest['algorithm']) => void; onDelete: (key: SSHKey) => Promise<void>; onToast: (message: string) => void }) {
  const [copying, setCopying] = useState<string | null>(null);
  async function copy(key: SSHKey) { setCopying(key.keyId); try { await navigator.clipboard.writeText(await publicKey(key.keyId)); onToast('Public key copied.'); } catch (error) { onToast((error as Error).message); } finally { setCopying(null); } }
  return <div className="keys-panel"><div className="manager-toolbar"><div><h2>SSH keys</h2><p className="panel-muted">Private key contents never leave the SSH directory.</p></div><div className="key-generate-actions"><button className="text-button" type="button" onClick={() => onGenerate('ed25519')}><KeyRound size={15} aria-hidden="true" /> Generate Ed25519</button><button className="primary" type="button" onClick={() => onGenerate('rsa')}><KeyRound size={15} aria-hidden="true" /> Generate RSA</button></div></div><div className="key-table" role="table" aria-label="SSH keys"><div className="key-table-head" role="row"><span>Filename</span><span>Algorithm</span><span>Fingerprint</span><span>Usage</span><span /></div>{keys.map((key) => <div className="key-table-row" role="row" key={key.keyId}><span><strong>{key.fileName}</strong><small>{key.readOnly ? 'read-only' : 'writable'} | {key.status}</small></span><span>{key.algorithm}{key.bits ? ` ${key.bits}` : ''}</span><code>{key.fingerprint || 'Unavailable'}</code><span>{instances.filter((instance) => instance.connectionDefinitionId && instance.sourceHostAlias).length ? 'referenced' : '-'}</span><span className="key-actions">{key.publicKeyAvailable && <button className="icon-button" type="button" disabled={copying === key.keyId} onClick={() => void copy(key)} aria-label={`Copy public key ${key.fileName}`} title="Copy public key"><Clipboard size={15} aria-hidden="true" /></button>}<button className="icon-button danger-icon" type="button" disabled={key.readOnly} onClick={() => void onDelete(key)} aria-label={`Delete SSH key ${key.fileName}`} title={key.readOnly ? 'Mounted key cannot be deleted' : 'Delete SSH key'}><Trash2 size={15} aria-hidden="true" /></button></span></div>)}{keys.length === 0 && <div className="manager-empty">No managed Ed25519 or RSA keys detected.</div>}</div></div>;
}

function DefinitionEditor({ editor, draft, keys, busy, onDraft, onSave, onClose }: { editor: Exclude<Editor, null>; draft: Draft; keys: SSHKey[]; busy: boolean; onDraft: (draft: Draft) => void; onSave: (event: React.FormEvent) => void; onClose: () => void }) {
  const set = (key: keyof Draft, value: string) => onDraft({ ...draft, [key]: value });
  return <Modal onClose={onClose}><form className="connection-editor" onSubmit={onSave}>
    <header><div><p className="eyebrow">STRUCTURED SSH CONFIG</p><h2>{editor.mode === 'create' ? 'New connection' : ('Edit ' + (editor.definition?.hostAlias || ''))}</h2></div><button className="icon-button" type="button" onClick={onClose} aria-label="Close editor"><X size={17} aria-hidden="true" /></button></header>
    <label>Host alias<input required pattern={'[A-Za-z0-9][\\-A-Za-z0-9._]{0,254}'} value={draft.hostAlias} onChange={(event) => set('hostAlias', event.target.value)} /></label>
    <label>HostName<input value={draft.hostName} onChange={(event) => set('hostName', event.target.value)} placeholder="destination hostname" /></label>
    <div className="form-grid"><label>User<input value={draft.user} onChange={(event) => set('user', event.target.value)} /></label><label>Port<input type="number" min="1" max="65535" value={draft.port} onChange={(event) => set('port', event.target.value)} /></label></div>
    <label>Identity files<div className="identity-options">{keys.map((key) => <label key={key.keyId}><input type="checkbox" checked={draft.identities.includes(key.fileName)} onChange={(event) => onDraft({ ...draft, identities: event.target.checked ? [...draft.identities, key.fileName] : draft.identities.filter((name) => name !== key.fileName) })} />{key.fileName}</label>)}</div></label>
    <div className="form-grid"><label>IdentitiesOnly<select value={draft.identitiesOnly} onChange={(event) => set('identitiesOnly', event.target.value)}><option value="">Unset</option><option value="yes">yes</option><option value="no">no</option></select></label><label>ServerAliveInterval<input type="number" min="0" max="4294967295" value={draft.serverAliveInterval} onChange={(event) => set('serverAliveInterval', event.target.value)} /></label></div>
    <div className="form-grid"><label>StrictHostKeyChecking<select value={draft.strictHostKeyChecking} onChange={(event) => set('strictHostKeyChecking', event.target.value)}><option value="">default</option><option value="no">no</option></select></label><label>UserKnownHostsFile<select value={draft.userKnownHostsFile} onChange={(event) => set('userKnownHostsFile', event.target.value)}><option value="">default</option><option value="/dev/null">/dev/null</option></select></label></div>
    <details className="advanced-options"><summary>Advanced connection options</summary><label className="checkbox-row"><input type="checkbox" checked={draft.tmuxEnabled} onChange={(event) => onDraft({ ...draft, tmuxEnabled: event.target.checked })} /> Enable tmux connection</label><label>Tmux session name<input disabled={!draft.tmuxEnabled} required={draft.tmuxEnabled} pattern="[A-Za-z][A-Za-z0-9_-]{0,63}" maxLength={64} value={draft.tmuxSessionName} onChange={(event) => set('tmuxSessionName', event.target.value)} placeholder="for example t" /></label><small className="field-help">The name is used by OpenSSH to attach or create the remote tmux session.</small></details>
    {(draft.strictHostKeyChecking === 'no' || draft.userKnownHostsFile === '/dev/null') && <div className="risk-warning" role="alert"><ShieldAlert size={16} aria-hidden="true" /> Host verification is weakened for this connection.</div>}
    <footer><button className="text-button" type="button" onClick={onClose}>Cancel</button><button className="primary" type="submit" disabled={busy}>{busy ? 'Saving...' : 'Save connection'}</button></footer>
  </form></Modal>;
}
function GenerationDialog({ value, existingAlgorithms, busy, onChange, onSubmit, onClose }: { value: GenerationRequest; existingAlgorithms: Set<string>; busy: boolean; onChange: (value: GenerationRequest) => void; onSubmit: (event: React.FormEvent) => void; onClose: () => void }) {
  return <Modal onClose={onClose}><form className="connection-editor" onSubmit={onSubmit}><header><div><p className="eyebrow">SSH KEYGEN</p><h2>Generate key</h2></div><button className="icon-button" type="button" onClick={onClose} aria-label="Close key generator"><X size={17} aria-hidden="true" /></button></header><label>Algorithm<select value={value.algorithm} onChange={(event) => onChange({ ...value, algorithm: event.target.value as GenerationRequest['algorithm'], fileName: event.target.value === 'rsa' ? 'id_rsa' : 'id_ed25519', rsaBits: event.target.value === 'rsa' ? 3072 : null })}><option value="ed25519" disabled={existingAlgorithms.has('ed25519')}>Ed25519{existingAlgorithms.has('ed25519') ? ' (already exists)' : ''}</option><option value="rsa" disabled={existingAlgorithms.has('rsa')}>RSA{existingAlgorithms.has('rsa') ? ' (already exists)' : ''}</option></select></label>{value.algorithm === 'rsa' && <label>RSA bits<select value={value.rsaBits || 3072} onChange={(event) => onChange({ ...value, rsaBits: Number(event.target.value) })}><option value="2048">2048</option><option value="3072">3072</option><option value="4096">4096</option></select></label>}<label>Filename<input required value={value.fileName} onChange={(event) => onChange({ ...value, fileName: event.target.value })} /></label><label>Comment <span className="optional">optional</span><input maxLength={255} value={value.comment} onChange={(event) => onChange({ ...value, comment: event.target.value })} /></label><div className="risk-warning"><KeyRound size={16} aria-hidden="true" /> Passphrase prompts appear only in the new key generation terminal.</div><footer><button className="text-button" type="button" onClick={onClose}>Cancel</button><button className="primary" type="submit" disabled={busy}><Play size={15} fill="currentColor" aria-hidden="true" /> {busy ? 'Starting...' : 'Open keygen terminal'}</button></footer></form></Modal>;
}
