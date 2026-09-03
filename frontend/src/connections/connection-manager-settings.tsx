import { useEffect } from 'react';
import { Bell, Filter, KeyRound, Monitor, Plus, RefreshCw, Settings, ShieldCheck } from 'lucide-react';
import { AuthSessionsPanel } from '../auth/auth-session-ui';
import { InterfaceSettings } from '../settings/interface-settings';
import { NotificationSettings } from '../settings/notification-settings';
import { SETTINGS_SECTIONS, type SettingsSection } from '../settings/settings-model';
import { ConnectionDefinitionEditor } from './connection-definition-editor';
import type { ConnectionManagerController, ConnectionManagerProps } from './connection-manager-controller';
import { ConnectionDefinitionRow, LocalConnectionRow, SourceBand } from './connection-manager-rows';
import { SSHKeyGenerationDialog } from './ssh-key-generation-dialog';
import { SSHKeysPanel } from './ssh-keys-panel';

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
  sessions: {
    eyebrow: 'LOGIN SESSIONS',
    title: 'Sessions',
    description: 'Review and revoke active login sessions for your Roaminal account.',
  },
};

const noop = () => undefined;
const noopAsync = async () => undefined;

type Props = ConnectionManagerProps & { controller: ConnectionManagerController };

export function SettingsPage({
  auth,
  connections,
  onConnect,
  appearance,
  onSaveAppearance,
  notificationState,
  onEnableNotifications,
  onDisableNotifications,
  onToast,
  section,
  onSectionChange,
  focusTarget,
  onFocusTargetConsumed,
  controller,
  authSessions = [],
  currentAuthSessionId = '',
  authSessionBusy = null,
  authSessionsLoading = false,
  onLoadAuthSessions = noopAsync,
  onRevokeAuthSession = noop,
  onLogoutOtherAuthSessions = noop,
}: Props) {
  const {
    appearanceDraft,
    setAppearanceDraft,
    definitions,
    keys,
    query,
    setQuery,
    sourcesLoading,
    definitionError,
    keyError,
    editorError,
    editor,
    draft,
    generation,
    setGeneration,
    settingsPage,
    settingsContent,
    sectionHeading,
    visible,
    optionsAvailable,
    optionsSource,
    tmuxDefinitions,
    definitionBusy,
    keyBusy,
    notificationSettings,
    saveSectionScroll,
    refreshSources,
    beginEditor,
    updateDraft,
    beginGeneration,
    saveDefinition,
    copyDefinition,
    removeDefinition,
    startGeneration,
    removeKey,
    setEditor,
  } = controller;
  const sectionCopy = SECTION_COPY[section];

  useEffect(() => {
    if (section === 'sessions') void onLoadAuthSessions();
  }, [onLoadAuthSessions, section]);

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
            const Icon = item.id === 'definitions' ? Settings : item.id === 'keys' ? KeyRound : item.id === 'interface' ? Monitor : item.id === 'notifications' ? Bell : ShieldCheck;
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
        {section === 'sessions' && <section className="settings-section-body" aria-labelledby="settings-section-title">
          <AuthSessionsPanel
            sessions={authSessions}
            currentId={currentAuthSessionId}
            busy={authSessionBusy}
            loading={authSessionsLoading}
            onRefresh={() => void onLoadAuthSessions()}
            onRevoke={onRevokeAuthSession}
            onLogoutOthers={onLogoutOtherAuthSessions}
          />
        </section>}
      </div>
      {editor && (
        <ConnectionDefinitionEditor
          editor={editor}
          draft={draft}
          keys={keys}
          busy={definitionBusy}
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
          busy={keyBusy}
          onChange={setGeneration}
          onSubmit={(event) => void startGeneration(event)}
          onClose={() => setGeneration(null)}
        />
      )}
    </section>
  );
}
