import type { DefinitionCollection } from './connection-api';

export function filterDefinitions(definitions: DefinitionCollection | null, query: string) {
  const normalized = query.trim().toLowerCase();
  return (definitions?.definitions || []).filter(
    (definition) =>
      !normalized ||
      `${definition.hostAlias || ''} ${definition.hostName || ''} ${definition.user || ''}`
        .toLowerCase()
        .includes(normalized),
  );
}

export function preserveLastOptions(previous: DefinitionCollection | null, next: DefinitionCollection): DefinitionCollection {
  const sourceStatus = next.tmuxOptionsSource?.status;
  if (!previous || !sourceStatus || sourceStatus === 'available' || sourceStatus === 'missing') return next;
  const previousByAlias = new Map(previous.definitions.filter((item) => item.hostAlias).map((item) => [item.hostAlias as string, item]));
  return {
    ...next,
    definitions: next.definitions.map((definition) => {
      const old = definition.hostAlias ? previousByAlias.get(definition.hostAlias) : undefined;
      if (!old) return definition;
      return { ...definition, tmux: old.tmux, filesystem: old.filesystem };
    }),
  };
}
