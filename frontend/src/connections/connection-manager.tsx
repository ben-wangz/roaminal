import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Bell, Filter, KeyRound, Monitor, Plus, RefreshCw, Settings } from 'lucide-react';
import type { AuthState } from '../auth/auth-storage';
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
import type { TerminalAppearance } from '../appearance/appearance-model';
import { InterfaceSettings } from '../settings/interface-settings';
import { NotificationSettings, useNotificationSettingsController } from '../settings/notification-settings';
import { SETTINGS_SECTIONS, type SettingsSection } from '../settings/settings-model';
import type { NotificationState } from '../status/notification-service';

type Props = {
  auth: AuthState | null;
  connections: ConnectionInstanceSummary[];
  onConnect: (definitionId: string, reuseFrom?: string, tmuxEnabled?: boolean) => Promise<boolean>;
  onGenerated: (instance: ConnectionInstanceSummary) => Promise<void>;
  appearance: TerminalAppearance;
  onSaveAppearance: (appearance: TerminalAppearance) => void;
  onSettingsDirtyChange: (dirty: boolean) => void;
  notificationState: NotificationState;
  onEnableNotifications: () => Promise<void>;
  onDisableNotifications: () => Promise<void>;
  onToast: (message: string, kind?: ToastKind) => void;
  section: SettingsSection;
  onSectionChange: (section: SettingsSection) => void;
  focusTarget: string | null;
  onFocusTargetConsumed: () => void;
};

const SECTION_COPY: Record<SettingsSection, { eyebrow: string; title: string; description: string }> = {
  definitions: {
    eyebrow: 'CONNECTION DEFINITIONS',
    title: 'Connection definitions',
    description: 'Manage saved SSH destinations and Roaminal connection options.',
  },
  keys: {
    eyebrow: 'SSH KEYS',
    title: 'SSH keys',
    description: 'Manage public-key metadata without exposing private key contents.',
  },
  interface: {
    eyebrow: 'INTERFACE',
    title: 'Interface',
    description: 'Tune terminal rendering and FileSystem presentation preferences.',
  },
  notifications: {
    eyebrow: 'NOTIFICATIONS',
    title: 'Notifications',
    description: 'Choose when Roaminal may notify you about Agent state changes.',
  },
};

export function ConnectionManager({ auth, connections, onConnect, onGenerated, appearance, onSaveAppearance, onSettingsDirtyChange, notificationState, onEnableNotifications, onDisableNotifications, onToast, section, onSectionChange, focusTarget, onFocusTargetConsumed }: Props) {
  const [definitions, setDefinitions] = useState<DefinitionCollection | null>(null);
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [etag, setETag] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [sourcesLoading, setSourcesLoading] = useState(false);
  const [definitionMutationBusy, setDefinitionMutationBusy] = useState(false);
  const [keyMutationBusy, setKeyMutationBusy] = useState(false);
  const [definitionError, setDefinitionError] = useState('');
  const [keyError, setKeyError] = useState('');
  const [editorError, setEditorError] = useState('');
  const [editor, setEditor] = useState<ConnectionEditor>(null);
  const [draft, setDraft] = useState<ConnectionDraft>(emptyDraft);
  const [appearanceDraft, setAppearanceDraft] = useState(appearance);
  const [generation, setGeneration] = useState<GenerationRequest | null>(null);
  const settingsPage = useRef<HTMLElement>(null);
  const settingsContent = useRef<HTMLDivElement>(null);
  const sectionHeading = useRef<HTMLHeadingElement>(null);
  const previousSection = useRef(section);
  const sectionScrollPositions = useRef<Partial<Record<SettingsSection, number>>>({});
  const refreshInFlight = useRef<Promise<void> | null>(null);
  const onToastRef = useRef(onToast);
  onToastRef.current = onToast;
  const notificationSettings = useNotificationSettingsController({
    auth,
    active: section === 'notifications',
    onToast,
  });

  useEffect(() => setAppearanceDraft(appearance), [appearance]);

  const appearanceDirty = appearanceDraft.fontId !== appearance.fontId || appearanceDraft.fontSize !== appearance.fontSize;
  useEffect(() => {
    onSettingsDirtyChange(appearanceDirty);
  }, [appearanceDirty, onSettingsDirtyChange]);

  const refreshSources = useCallback(async () => {
    if (refreshInFlight.current) return refreshInFlight.current;
    setSourcesLoading(true);
    const request = Promise.allSettled([loadDefinitions(), loadKeys()]).then(([definitionResult, keyResult]) => {
      if (definitionResult.status === 'fulfilled') {
        setDefinitionError('');
        setDefinitions((previous) => preserveLastOptions(previous, definitionResult.value.data));
        setETag(definitionResult.value.etag);
      } else {
        const message = (definitionResult.reason as Error).message;
        setDefinitionError(message);
        onToastRef.current(`Connection definitions: ${message}`, 'error');
      }
      if (keyResult.status === 'fulfilled') {
        setKeyError('');
        setKeys(keyResult.value.keys);
      } else {
        const message = (keyResult.reason as Error).message;
        setKeyError(message);
        onToastRef.current(`SSH keys: ${message}`, 'error');
      }
    }).finally(() => {
      refreshInFlight.current = null;
      setSourcesLoading(false);
    });
    refreshInFlight.current = request;
    return request;
  }, []);

  useEffect(() => {
    void refreshSources();
  }, [refreshSources]);

  useEffect(() => {
    const scrollElement = typeof window !== 'undefined' && window.matchMedia('(max-width: 800px)').matches
      ? settingsPage.current
      : settingsContent.current;
    if (!scrollElement) return;
    const position = sectionScrollPositions.current[section] || 0;
    window.requestAnimationFrame(() => {
      scrollElement.scrollTop = position;
    });
  }, [section]);

  useEffect(() => {
    if (previousSection.current === section) return;
    previousSection.current = section;
    if (focusTarget) return;
    window.requestAnimationFrame(() => sectionHeading.current?.focus({ preventScroll: true }));
  }, [focusTarget, section]);

  const visible = useMemo(() => filterDefinitions(definitions, query), [definitions, query]);

  const optionsAvailable = !definitions?.tmuxOptionsSource || definitions.tmuxOptionsSource.status === 'missing' || definitions.tmuxOptionsSource.status === 'available';

  function saveSectionScroll() {
    const scrollElement = typeof window !== 'undefined' && window.matchMedia('(max-width: 800px)').matches
      ? settingsPage.current
      : settingsContent.current;
    if (scrollElement) sectionScrollPositions.current[section] = scrollElement.scrollTop;
  }

  function beginEditor(mode: 'create' | 'edit', definition?: ConnectionDefinition) {
    setEditorError('');
    setEditor({ mode, definition });
    setDraft(draftFrom(definition, keys));
  }

  function updateDraft(next: ConnectionDraft) {
    setEditorError('');
    setDraft(next);
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
    setEditorError('');
    if (!etag) {
      setEditorError('Config ETag unavailable; refresh the source before saving.');
      return;
    }
    const duplicate = definitions?.definitions.some((definition) => definition.type === 'ssh'
      && definition.connectionDefinitionId !== editor?.definition?.connectionDefinitionId
      && definition.hostAlias?.trim().toLowerCase() === draft.hostAlias.trim().toLowerCase());
    if (duplicate) {
      setEditorError('Connection name must be unique.');
      return;
    }
    setDefinitionMutationBusy(true);
    try {
      const result =
        editor?.mode === 'edit' && editor.definition
          ? await updateDefinition(editor.definition.connectionDefinitionId, bodyFrom(draft), etag)
          : await createDefinition(bodyFrom(draft), etag);
      setDefinitions(result.data);
      setETag(result.etag);
      setEditor(null);
    } catch (error) {
      setEditorError((error as Error).message);
      onToast((error as Error).message, 'error');
    } finally {
      setDefinitionMutationBusy(false);
    }
  }

  async function copyDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias) return;
    const alias = window.prompt('New connection name', `${definition.hostAlias}-copy`);
    if (!alias) return;
    setDefinitionMutationBusy(true);
    try {
      const result = await duplicateDefinition(definition.connectionDefinitionId, alias.trim(), etag);
      setDefinitions(result.data);
      setETag(result.etag);
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setDefinitionMutationBusy(false);
    }
  }

  async function removeDefinition(definition: ConnectionDefinition) {
    if (!etag || !definition.hostAlias || !window.confirm(`Delete connection ${definition.hostAlias}?`)) return;
    setDefinitionMutationBusy(true);
    try {
      const result = await deleteDefinition(definition.connectionDefinitionId, etag);
      setDefinitions(result.data);
      setETag(result.etag);
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setDefinitionMutationBusy(false);
    }
  }

  async function startGeneration(event: React.FormEvent) {
    event.preventDefault();
    if (!generation) return;
    setKeyMutationBusy(true);
    try {
      const instance = await generateKey(generation);
      setGeneration(null);
      await onGenerated(instance);
      onToast(`Key generation connection ${instance.connectionInstanceId.slice(0, 8)} is ready.`, 'success');
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setKeyMutationBusy(false);
    }
  }

  async function removeKey(key: SSHKey) {
    const referenceCount = definitions?.definitions.filter((definition) => definition.identityFileNames.includes(key.fileName)).length || 0;
    if (key.readOnly) return;
    const message = referenceCount > 0
      ? `${key.fileName} is referenced by ${referenceCount} connection definition${referenceCount === 1 ? '' : 's'}. Delete it anyway?`
      : `Delete SSH key ${key.fileName} and its public key?`;
    if (!window.confirm(message)) return;
    setKeyMutationBusy(true);
    try {
      await deleteKey(key.keyId);
      setKeys((current) => current.filter((item) => item.keyId !== key.keyId));
      onToast(`Deleted ${key.fileName}.`, 'success');
    } catch (error) {
      onToast((error as Error).message, 'error');
    } finally {
      setKeyMutationBusy(false);
    }
  }

  const sectionCopy = SECTION_COPY[section];
  const definitionBusy = sourcesLoading || definitionMutationBusy;
  const keyBusy = sourcesLoading || keyMutationBusy;
  const optionsSource = definitions?.tmuxOptionsSource || { status: 'missing', readable: false, writable: false };
  const tmuxDefinitions = definitions?.definitions.filter((definition) => definition.type === 'ssh' && definition.tmux?.enabled) || [];

  return (
    <section ref={settingsPage} className="settings-page" aria-label="Settings" onScroll={saveSectionScroll}>
      <header className="settings-section-header">
        <p className="eyebrow">{sectionCopy.eyebrow}</p>
        <h1 ref={sectionHeading} id="settings-section-title" tabIndex={-1}>{sectionCopy.title}</h1>
        <p>{sectionCopy.description}</p>
      </header>
      <aside className="settings-navigation">
        <p className="settings-navigation-eyebrow">SETTINGS</p>
        <nav aria-label="Settings sections">
          {SETTINGS_SECTIONS.map((item) => {
            const Icon = item.id === 'definitions' ? Settings : item.id === 'keys' ? KeyRound : item.id === 'interface' ? Monitor : Bell;
            const active = section === item.id;
            return (
              <button
                key={item.id}
                className={`settings-navigation-item ${active ? 'active' : ''}`}
                type="button"
                aria-current={active ? 'page' : undefined}
                onClick={() => { saveSectionScroll(); onFocusTargetConsumed(); onSectionChange(item.id); }}
                data-testid={`settings-section-${item.id}`}
              >
                <Icon size={19} aria-hidden="true" />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </aside>
      <div ref={settingsContent} className="settings-content" onScroll={saveSectionScroll}>
        {section === 'definitions' && <section className="settings-section-body" aria-labelledby="settings-section-title">
          <div className="settings-source-grid">
            {definitions ? <>
              <SourceBand source={definitions.configSource} label="SSH config" error={definitionError} />
              <SourceBand source={optionsSource} label="Roaminal tmux" error={definitionError} />
              <SourceBand source={optionsSource} label="FileSystem options" error={definitionError} />
            </> : definitionError ? <div className="settings-source-error" role="alert">Unable to load connection definitions: {definitionError}</div> : <div className="settings-loading" role="status">Loading connection definitions...</div>}
          </div>
          <div className="settings-toolbar">
            <label className="settings-search">
              <Filter size={18} aria-hidden="true" />
              <span className="sr-only">Filter connections</span>
              <input
                id="connection-manager-filter"
                name="connectionFilter"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Alias or destination"
                aria-label="Filter connections"
              />
            </label>
            <button className="settings-secondary-action" type="button" onClick={() => void refreshSources()} disabled={definitionBusy}>
              <RefreshCw size={17} aria-hidden="true" className={sourcesLoading ? 'spin' : ''} /> Refresh
            </button>
            <button className="primary settings-add-action" type="button" onClick={() => beginEditor('create')} disabled={!definitions?.configSource.writable || definitionBusy}>
              <Plus size={17} aria-hidden="true" /> Add connection
            </button>
          </div>
          {definitionError && definitions && <div className="settings-inline-error" role="alert">The last definition refresh failed: {definitionError}. Showing the last successful data.</div>}
          <div className="connection-list settings-definition-list" role="table" aria-label="Connection definitions">
            <div className="settings-definition-list-header" role="row">
                <span role="columnheader">Connection</span>
                <span role="columnheader">Managed keys</span>
                <span role="columnheader">Trust</span>
                <span role="columnheader">Tmux</span>
                <span role="columnheader">Actions</span>
            </div>
            <LocalConnectionRow onConnect={() => void onConnect('local')} />
            {visible
              .filter((definition) => definition.type === 'ssh')
              .map((definition) => (
                <ConnectionDefinitionRow
                  key={definition.connectionDefinitionId}
                  definition={definition}
                  editable={Boolean(definitions?.configSource.writable)}
                  busy={definitionBusy}
                  connections={connections}
                  onConnect={onConnect}
                  onEdit={() => beginEditor('edit', definition)}
                  onDuplicate={() => void copyDefinition(definition)}
                  onDelete={() => void removeDefinition(definition)}
                />
              ))}
            {!visible.some((definition) => definition.type === 'ssh') && definitions && (
              <div className="manager-empty">No matching SSH definitions.</div>
            )}
          </div>
          {definitions && <p className="settings-definition-count">{visible.length + 1} connections</p>}
        </section>}
        {section === 'keys' && <section className="settings-section-body" aria-labelledby="settings-section-title">
          {keyError && <div className="settings-inline-error" role="alert">The last SSH key refresh failed: {keyError}. Showing the last successful data.</div>}
          <SSHKeysPanel
            keys={keys}
            definitions={definitions?.definitions || []}
            busy={keyBusy}
            onRefresh={() => void refreshSources()}
            onGenerate={beginGeneration}
            onDelete={removeKey}
            onToast={onToast}
          />
        </section>}
        {section === 'interface' && <section className="settings-section-body" aria-labelledby="settings-section-title">
          <InterfaceSettings
            appearance={appearance}
            appearanceDraft={appearanceDraft}
            onAppearanceDraftChange={setAppearanceDraft}
            onSaveAppearance={onSaveAppearance}
          />
        </section>}
        {section === 'notifications' && <section className="settings-section-body" aria-labelledby="settings-section-title">
          <NotificationSettings
            auth={auth}
            definitions={tmuxDefinitions}
            preferences={notificationSettings.preferences}
            loading={notificationSettings.loading}
            busyKeys={notificationSettings.busyKeys}
            onUpdatePreference={notificationSettings.updatePreference}
            notificationState={notificationState}
            onEnableNotifications={onEnableNotifications}
            onDisableNotifications={onDisableNotifications}
            focusTarget={focusTarget}
            onFocusTargetConsumed={onFocusTargetConsumed}
          />
        </section>}
      </div>
      {editor && (
        <ConnectionDefinitionEditor
          editor={editor}
          draft={draft}
          keys={keys}
          busy={definitionMutationBusy}
          optionsAvailable={optionsAvailable}
          error={editorError}
          onDraft={updateDraft}
          onSave={(event) => void saveDefinition(event)}
          onRefresh={() => void refreshSources()}
          onClose={() => setEditor(null)}
        />
      )}
      {generation && (
        <SSHKeyGenerationDialog
          value={generation}
          existingAlgorithms={new Set(keys.map((key) => key.algorithm))}
          busy={keyMutationBusy}
          onChange={setGeneration}
          onSubmit={(event) => void startGeneration(event)}
          onClose={() => setGeneration(null)}
        />
      )}
    </section>
  );
}
