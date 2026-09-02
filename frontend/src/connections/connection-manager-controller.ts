import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { AuthState } from '../auth/auth-storage';
import type { TerminalAppearance } from '../appearance/appearance-model';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { ToastKind } from '../ui/toast';
import type { NotificationState } from '../status/notification-service';
import {
  loadDefinitions,
  loadKeys,
  type DefinitionCollection,
  type GenerationRequest,
  type SSHKey,
} from './connection-api';
import {
  emptyDraft,
  type ConnectionDraft,
  type ConnectionEditor,
} from './connection-definition-model';
import { filterDefinitions, preserveLastOptions } from './connection-manager-state';
import { useNotificationSettingsController } from '../settings/notification-settings';
import type { SettingsSection } from '../settings/settings-model';
import { useConnectionManagerActions } from './connection-manager-actions';

export type ConnectionManagerProps = {
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

export function useConnectionManagerController({
  auth,
  appearance,
  onSettingsDirtyChange,
  onToast,
  section,
  focusTarget,
  onGenerated,
}: ConnectionManagerProps) {
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
  const optionsAvailable = !definitions?.tmuxOptionsSource
    || definitions.tmuxOptionsSource.status === 'missing'
    || definitions.tmuxOptionsSource.status === 'available';

  function saveSectionScroll() {
    const scrollElement = typeof window !== 'undefined' && window.matchMedia('(max-width: 800px)').matches
      ? settingsPage.current
      : settingsContent.current;
    if (scrollElement) sectionScrollPositions.current[section] = scrollElement.scrollTop;
  }

  const optionsSource = definitions?.tmuxOptionsSource || { status: 'missing', readable: false, writable: false };
  const tmuxDefinitions = definitions?.definitions.filter((definition) => definition.type === 'ssh' && definition.tmux?.enabled) || [];
  const actions = useConnectionManagerActions({
    definitions,
    keys,
    etag,
    editor,
    draft,
    generation,
    onGenerated,
    onToast,
    setDefinitions,
    setETag,
    setKeys,
    setEditor,
    setDraft,
    setGeneration,
    setEditorError,
    setDefinitionMutationBusy,
    setKeyMutationBusy,
  });

  return {
    appearanceDraft,
    setAppearanceDraft,
    appearanceDirty,
    definitions,
    keys,
    draft,
    query,
    setQuery,
    sourcesLoading,
    definitionError,
    keyError,
    editorError,
    editor,
    generation,
    setGeneration,
    settingsPage,
    settingsContent,
    sectionHeading,
    visible,
    optionsAvailable,
    optionsSource,
    tmuxDefinitions,
    definitionBusy: sourcesLoading || definitionMutationBusy,
    keyBusy: sourcesLoading || keyMutationBusy,
    notificationSettings,
    saveSectionScroll,
    refreshSources,
    ...actions,
    setEditor,
  };
}

export type ConnectionManagerController = ReturnType<typeof useConnectionManagerController>;
