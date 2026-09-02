import type { Dispatch, FormEvent, SetStateAction } from 'react';
import {
  bodyFrom,
  draftFrom,
  type ConnectionDraft,
  type ConnectionEditor,
} from './connection-definition-model';
import {
  createDefinition,
  deleteDefinition,
  deleteKey,
  duplicateDefinition,
  generateKey,
  updateDefinition,
  type ConnectionDefinition,
  type DefinitionCollection,
  type GenerationRequest,
  type SSHKey,
} from './connection-api';
import type { ToastKind } from '../ui/toast';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';

type Params = {
  definitions: DefinitionCollection | null;
  keys: SSHKey[];
  etag: string | null;
  editor: ConnectionEditor;
  draft: ConnectionDraft;
  generation: GenerationRequest | null;
  onGenerated: (instance: ConnectionInstanceSummary) => Promise<void>;
  onToast: (message: string, kind?: ToastKind) => void;
  setDefinitions: Dispatch<SetStateAction<DefinitionCollection | null>>;
  setETag: Dispatch<SetStateAction<string | null>>;
  setKeys: Dispatch<SetStateAction<SSHKey[]>>;
  setEditor: Dispatch<SetStateAction<ConnectionEditor>>;
  setDraft: Dispatch<SetStateAction<ConnectionDraft>>;
  setGeneration: Dispatch<SetStateAction<GenerationRequest | null>>;
  setEditorError: Dispatch<SetStateAction<string>>;
  setDefinitionMutationBusy: Dispatch<SetStateAction<boolean>>;
  setKeyMutationBusy: Dispatch<SetStateAction<boolean>>;
};

export function useConnectionManagerActions({
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
}: Params) {
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

  async function saveDefinition(event: FormEvent) {
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
      const result = editor?.mode === 'edit' && editor.definition
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

  async function startGeneration(event: FormEvent) {
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

  return { beginEditor, updateDraft, beginGeneration, saveDefinition, copyDefinition, removeDefinition, startGeneration, removeKey };
}
