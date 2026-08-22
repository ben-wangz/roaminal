import { memo, useEffect, useMemo, useRef, useState } from 'react';
import { Bot, Check, ChevronDown, ChevronRight, FolderOpen, FolderPlus, GripVertical, PanelLeftClose, PanelLeftOpen, Search, X } from 'lucide-react';
import type { ConnectionInstanceLayout, InstanceMovePlacement } from '../connections/connection-instance-groups';
import { groupedConnectionInstances, UNGROUPED_GROUP_ID } from '../connections/connection-instance-groups';
import type { ConnectionInstanceSummary } from '../terminal/terminal-protocol';
import type { TerminalRuntime } from '../terminal/terminal-runtime';
import { ContextualKeyboard } from '../input/contextual-keyboard';
import { SIDEBAR_BREAKPOINT_QUERY } from '../input/viewport';
import type { ContextualMode } from '../input/contextual-keyboard-model';
import { ConnectionActions, type ConnectionGroupMoveTarget } from './connection-actions';
import { TerminalPreview, type TerminalPreviewRuntime } from '../terminal/terminal-preview';
import { useConnectionGroupReorder } from './use-connection-group-reorder';
import { agentSummary, agentTitle } from '../agent/agent-api';
import type { WorkspaceMode } from '../app/workspace-page';
import { ConnectionGroupActions } from './connection-group-actions';
import { loadCollapsed, saveCollapsed } from './connection-sidebar-storage';

type Props = {
  id: string;
  connections: ConnectionInstanceSummary[];
  layout: ConnectionInstanceLayout;
  loginSessionId: string;
  active: string | null;
  open: boolean;
  previewConnectionInstanceId: string | null;
  previewRuntime: TerminalPreviewRuntime | null;
  onToggle: () => void;
  onSelect: (id: string) => void;
  onMoveInstance: (id: string, groupId: string, targetId: string | null, placement: InstanceMovePlacement) => Promise<void>;
  onReorderGroup: (id: string, targetId: string, placement: InstanceMovePlacement) => Promise<void>;
  onCreateGroup: (name: string) => Promise<boolean>;
  onRenameGroup: (id: string, name: string) => Promise<boolean>;
  onDeleteGroup: (id: string) => Promise<boolean>;
  onMoveGroupMembers: (id: string) => Promise<void>;
  onPreviewStart: (id: string) => void;
  onPreviewEnd: (id: string) => void;
  onAgent: (id: string) => void;
  onOpenFileSystem: (id: string) => void;
  workspaceMode: WorkspaceMode;
  onRename: (id: string) => void;
  onAutomaticTitle: (id: string) => void;
  onTerminate: (id: string) => void;
  activeInstance: ConnectionInstanceSummary | null;
  activeRuntime: TerminalRuntime | null;
  contextualMode: ContextualMode;
  onContextualModeChange: (mode: ContextualMode) => void;
};

export function shortConnectionId(id: string): string {
  const part = id.split('-').pop();
  return part && part.length >= 12 ? part.slice(-12) : id.slice(0, 12);
}

export function sinceLabel(createdAt: string): string {
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) return 'Unknown';
  const pad = (value: number) => String(value).padStart(2, '0');
  const hour = date.getHours();
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(hour % 12 || 12)}:${pad(date.getMinutes())} ${hour >= 12 ? 'PM' : 'AM'}`;
}

function connectionStateLabel(connection: ConnectionInstanceSummary): string {
  if (connection.attention) return 'Activity waiting';
  if (connection.purpose === 'ssh_key_generation') return 'SSH key generation';
  return connection.type === 'ssh' ? 'SSH connection' : 'Local connection';
}

function connectionPathLabel(connection: ConnectionInstanceSummary): string | null {
  if (connection.purpose === 'ssh_key_generation') return `TARGET: ${connection.title || 'key'}`;
  const cwd = connection.cwd?.trim();
  return cwd ? `PWD: ${cwd}` : null;
}

function canPreview(): boolean {
  return window.matchMedia('(pointer: fine)').matches && !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches;
}

function matchesSearch(connection: ConnectionInstanceSummary, groupName: string, query: string): boolean {
  if (!query) return true;
  const value = [groupName, connection.title, connection.connectionInstanceId, connection.cwd, connection.type, connection.sourceHostAlias].filter(Boolean).join(' ').toLowerCase();
  return value.includes(query);
}

export const ConnectionSidebar = memo(function ConnectionSidebar({
  id,
  connections,
  layout,
  loginSessionId,
  active,
  open,
  previewConnectionInstanceId,
  previewRuntime,
  onToggle,
  onSelect,
  onMoveInstance,
  onReorderGroup,
  onCreateGroup,
  onRenameGroup,
  onDeleteGroup,
  onMoveGroupMembers,
  onPreviewStart,
  onPreviewEnd,
  onAgent,
  onOpenFileSystem,
  workspaceMode,
  onRename,
  onAutomaticTitle,
  onTerminate,
  activeInstance,
  activeRuntime,
  contextualMode,
  onContextualModeChange,
}: Props) {
  const aside = useRef<HTMLElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const mounted = useRef(false);
  const [search, setSearch] = useState('');
  const [collapsed, setCollapsed] = useState<Set<string>>(() => loadCollapsed(loginSessionId));
  const [creatingGroup, setCreatingGroup] = useState(false);
  const [newGroupName, setNewGroupName] = useState('');
  const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
  const [editingGroupName, setEditingGroupName] = useState('');
  const [dragExpandedGroupId, setDragExpandedGroupId] = useState<string | null>(null);
  const groupHeaderRefs = useRef(new Map<string, HTMLButtonElement>());
  const previousGroupIds = useRef<string[]>([]);
  const focusNewGroup = useRef(false);
  const groups = useMemo(() => groupedConnectionInstances(layout, connections), [connections, layout]);
  const query = search.trim().toLowerCase();
  const {
    capacityBlockedGroupId, clearDrag, draggedConnectionInstanceId, draggedGroupId, dragLeave, dragOverGroup, dragOverInstance, dropGroup, dropInstance, dropTarget, moveGroupWithKeyboard, moveInstanceWithKeyboard, reorderPending, startGroupDrag, startInstanceDrag,
  } = useConnectionGroupReorder({ layout, disabled: Boolean(query), onMoveInstance, onReorderGroup, onPreviewEnd, onExpandGroup: setDragExpandedGroupId });

  useEffect(() => {
    setCollapsed(loadCollapsed(loginSessionId));
  }, [loginSessionId]);
  useEffect(() => {
    const valid = new Set(groups.map(({ group }) => group.groupId));
    setCollapsed((current) => {
      const next = new Set([...current].filter((groupId) => valid.has(groupId)));
      if (next.size !== current.size) saveCollapsed(loginSessionId, next);
      return next.size === current.size ? current : next;
    });
  }, [groups, loginSessionId]);
  useEffect(() => {
    if (!draggedConnectionInstanceId && !draggedGroupId) setDragExpandedGroupId(null);
  }, [draggedConnectionInstanceId, draggedGroupId]);
  useEffect(() => {
    const currentIds = groups.map(({ group }) => group.groupId);
    if (focusNewGroup.current) {
      const previous = new Set(previousGroupIds.current);
      const created = groups.find(({ group }) => group.groupId !== UNGROUPED_GROUP_ID && !previous.has(group.groupId));
      if (created) {
        focusNewGroup.current = false;
        window.requestAnimationFrame(() => groupHeaderRefs.current.get(created.group.groupId)?.focus());
      }
    }
    previousGroupIds.current = currentIds;
  }, [groups]);
  useEffect(() => {
    if (mounted.current && open) toggle.current?.focus();
    mounted.current = true;
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const handleKeyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches) {
        event.preventDefault();
        onToggle();
        return;
      }
      if (event.key !== 'Tab' || !window.matchMedia(SIDEBAR_BREAKPOINT_QUERY).matches || !aside.current) return;
      const focusable = Array.from(aside.current.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'));
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!aside.current.contains(document.activeElement)) {
        event.preventDefault();
        first.focus();
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', handleKeyboard);
    return () => document.removeEventListener('keydown', handleKeyboard);
  }, [onToggle, open]);

  const toggleGroup = (groupId: string) => {
    const next = new Set(collapsed);
    if (next.has(groupId)) next.delete(groupId);
    else next.add(groupId);
    setCollapsed(next);
    saveCollapsed(loginSessionId, next);
  };
  const submitNewGroup = async () => {
    const value = newGroupName.trim();
    if (!value) return;
    const saved = await onCreateGroup(value);
    if (!saved) return;
    focusNewGroup.current = true;
    setNewGroupName('');
    setCreatingGroup(false);
  };
  const submitRename = async () => {
    if (!editingGroupId || !editingGroupName.trim()) return;
    const groupId = editingGroupId;
    const saved = await onRenameGroup(groupId, editingGroupName.trim());
    if (!saved) return;
    setEditingGroupId(null);
    window.requestAnimationFrame(() => groupHeaderRefs.current.get(groupId)?.focus());
  };

  return (
    <>
      {open && <button className="sidebar-backdrop" type="button" aria-label="Close sidebar" onClick={onToggle} />}
      <aside ref={aside} id={id} className={`sidebar ${open ? 'open' : 'closed'}`} aria-hidden={!open} inert={!open || undefined}>
        <div className="sidebar-header">
          <div className="brand-mark small">r<span>&gt;</span></div>
          <strong>Roaminal</strong>
          <button ref={toggle} className="icon-button sidebar-toggle" type="button" onClick={onToggle} aria-label="Toggle sidebar" title="Toggle sidebar" aria-expanded={open} aria-controls={id}>
            {open ? <PanelLeftClose aria-hidden="true" size={18} /> : <PanelLeftOpen aria-hidden="true" size={18} />}
          </button>
        </div>
        <div className="connection-sidebar-toolbar">
          <strong>Connections <span>{connections.length}</span></strong>
          <button className="icon-button" type="button" aria-label="Create connection group" title="Create connection group" onClick={() => setCreatingGroup(true)}><FolderPlus size={15} aria-hidden="true" /></button>
        </div>
        <label className="connection-sidebar-search"><Search size={14} aria-hidden="true" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search connections" aria-label="Search connections" /></label>
        {creatingGroup && <div className="connection-group-create"><input autoFocus value={newGroupName} onChange={(event) => setNewGroupName(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void submitNewGroup(); if (event.key === 'Escape') setCreatingGroup(false); }} placeholder="Group name" aria-label="New group name" /><button className="icon-button" type="button" title="Create group" aria-label="Create group" onClick={() => void submitNewGroup()}><Check size={14} aria-hidden="true" /></button><button className="icon-button" type="button" title="Cancel" aria-label="Cancel" onClick={() => setCreatingGroup(false)}><X size={14} aria-hidden="true" /></button></div>}
        <div className="connection-list connection-group-list">
          {groups.map(({ group, connections: groupConnections }) => {
            const matchingConnections = groupConnections.filter((connection) => matchesSearch(connection, group.name, query));
            const groupMatches = query && group.name.toLowerCase().includes(query);
            if (query && !groupMatches && matchingConnections.length === 0) return null;
            const isCollapsed = !query && collapsed.has(group.groupId) && dragExpandedGroupId !== group.groupId;
            const editing = editingGroupId === group.groupId;
            const groupDrop = dropTarget?.kind === 'group' && dropTarget.id === group.groupId;
            const groupMembersId = `connection-group-members-${group.groupId}`;
            return (
              <section key={group.groupId} className={`connection-group ${isCollapsed ? 'collapsed' : ''} ${groupDrop ? `drop-${dropTarget?.placement}` : ''} ${capacityBlockedGroupId === group.groupId ? 'drop-blocked' : ''}`} title={capacityBlockedGroupId === group.groupId ? 'Group limit reached (10)' : undefined} onDragOver={(event) => dragOverGroup(event, group.groupId)} onDragLeave={(event) => dragLeave(event, group.groupId)} onDrop={(event) => dropGroup(event, group.groupId)}>
                <header className="connection-group-header">
                  <button className="connection-group-toggle" type="button" aria-label={isCollapsed ? `Expand ${group.name}` : `Collapse ${group.name}`} aria-expanded={!isCollapsed} aria-controls={groupMembersId} onClick={() => toggleGroup(group.groupId)}>{isCollapsed ? <ChevronRight size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}</button>
                  {editing ? <div className="connection-group-edit"><input autoFocus value={editingGroupName} onChange={(event) => setEditingGroupName(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void submitRename(); if (event.key === 'Escape') setEditingGroupId(null); }} aria-label={`Rename ${group.name}`} /><button className="icon-button" type="button" title="Save group name" aria-label="Save group name" onClick={() => void submitRename()}><Check size={13} aria-hidden="true" /></button><button className="icon-button" type="button" title="Cancel" aria-label="Cancel" onClick={() => setEditingGroupId(null)}><X size={13} aria-hidden="true" /></button></div> : <button ref={(element) => { if (element) groupHeaderRefs.current.set(group.groupId, element); else groupHeaderRefs.current.delete(group.groupId); }} className="connection-group-name" type="button" onClick={() => toggleGroup(group.groupId)}>{group.name}</button>}
                  <span className="connection-group-count" aria-label={`${groupConnections.length} connection instances`}>{groupConnections.length}</span>
                  <button className="connection-group-drag-handle" type="button" draggable={!reorderPending && !query} aria-label={`Reorder ${group.name} group`} title={query ? 'Clear search to reorder groups' : 'Reorder group'} onClick={(event) => event.stopPropagation()} onDragStart={(event) => startGroupDrag(event, group.groupId)} onDragEnd={clearDrag} onKeyDown={(event) => moveGroupWithKeyboard(event, group.groupId)}><GripVertical size={14} aria-hidden="true" /></button>
                  {group.groupId !== UNGROUPED_GROUP_ID && <ConnectionGroupActions nonEmpty={groupConnections.length > 0} onRename={() => { setEditingGroupId(group.groupId); setEditingGroupName(group.name); }} onMoveAll={() => void onMoveGroupMembers(group.groupId)} onDelete={() => void onDeleteGroup(group.groupId)} />}
                </header>
                {!isCollapsed && <div id={groupMembersId} className="connection-group-members" role="group" aria-label={`${group.name} connections`}>
                  {(groupMatches ? groupConnections : matchingConnections).map((connection) => renderConnection(connection, group.groupId))}
                  {groupConnections.length === 0 && <div className="connection-group-empty">No connections</div>}
                </div>}
                {capacityBlockedGroupId === group.groupId && <div className="connection-group-capacity-hint" role="status">Group limit reached (10)</div>}
              </section>
            );
          })}
          {!groups.some(({ group, connections: groupConnections }) => !query || group.name.toLowerCase().includes(query) || groupConnections.some((connection) => matchesSearch(connection, group.name, query))) && <div className="connection-group-empty">No matching connections</div>}
        </div>
        <ContextualKeyboard instance={activeInstance} runtime={activeRuntime} mode={contextualMode} onModeChange={onContextualModeChange} />
        <div className="sidebar-footer">Connection workspace</div>
      </aside>
    </>
  );

  function renderConnection(connection: ConnectionInstanceSummary, groupId: string) {
    const previewing = previewConnectionInstanceId === connection.connectionInstanceId && Boolean(previewRuntime);
    const pathLabel = connectionPathLabel(connection);
    const startPreview = () => {
      if (workspaceMode === 'terminal' && !draggedConnectionInstanceId && !draggedGroupId && !reorderPending && canPreview()) onPreviewStart(connection.connectionInstanceId);
    };
    const stopPreview = () => onPreviewEnd(connection.connectionInstanceId);
    const dropPlacement = dropTarget?.kind === 'instance' && dropTarget.id === connection.connectionInstanceId ? dropTarget.placement : null;
    const agent = agentSummary(connection);
    const terminalNavigation = workspaceMode === 'filesystem';
    const agentDisabled = !terminalNavigation && (agent.support !== 'supported' || agent.component === 'initializing');
    const agentLabel = terminalNavigation ? 'Open Terminal' : agentTitle(agent);
    return (
      <article className={`connection-card ${connection.connectionInstanceId === active ? 'active' : ''} ${connection.attention ? 'attention' : ''} ${previewing ? 'previewing' : ''} ${draggedConnectionInstanceId === connection.connectionInstanceId ? 'dragging' : ''} ${dropPlacement ? `drop-${dropPlacement}` : ''}`} data-connection-id={connection.connectionInstanceId} key={connection.connectionInstanceId} onMouseEnter={startPreview} onMouseLeave={stopPreview} onClick={() => onSelect(connection.connectionInstanceId)} onDragOver={(event) => dragOverInstance(event, connection.connectionInstanceId, groupId)} onDragLeave={(event) => dragLeave(event, connection.connectionInstanceId)} onDrop={(event) => dropInstance(event, connection.connectionInstanceId, groupId)} onFocus={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) startPreview(); }} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) stopPreview(); }}>
        <div className="connection-card-preview">{previewing && previewRuntime && <TerminalPreview runtime={previewRuntime} />}</div>
        <div className="connection-card-overlay">
          <button className="connection-select" type="button" onClick={() => onSelect(connection.connectionInstanceId)} aria-current={connection.connectionInstanceId === active ? 'page' : undefined} title={connection.connectionInstanceId}><span className="connection-indicator" /><span className="connection-title-wrap"><b>{connection.title || 'Connection'}</b><small>{connectionStateLabel(connection)}</small></span></button>
          <div className="connection-metadata"><span>ID: {shortConnectionId(connection.connectionInstanceId)}</span>{pathLabel && <span className="connection-path" title={connection.cwd}>{pathLabel}</span>}<time dateTime={connection.createdAt} title={connection.createdAt}>SINCE: {sinceLabel(connection.createdAt)}</time></div>
        </div>
        <div className="connection-actions" aria-label="Connection extensions and actions">
          <button className="connection-drag-handle" type="button" draggable={!reorderPending && !query} aria-label={`Reorder ${connection.title || 'connection'}`} title={query ? 'Clear search to reorder connections' : 'Reorder connection'} onClick={(event) => event.stopPropagation()} onDragStart={(event) => startInstanceDrag(event, connection.connectionInstanceId)} onDragEnd={clearDrag} onKeyDown={(event) => moveInstanceWithKeyboard(event, connection.connectionInstanceId, groupId)}><GripVertical aria-hidden="true" size={15} /></button>
          <button className={`extension-button agent-extension agent-status-${agent.component} agent-activity-${agent.activity}`} type="button" aria-label={agentLabel} aria-disabled={agentDisabled} disabled={agentDisabled} data-agent-state={agent.component} title={agentLabel} onClick={(event) => { event.stopPropagation(); onAgent(connection.connectionInstanceId); }}><Bot aria-hidden="true" size={15} /></button>
          <button className="extension-button" type="button" aria-label="Files extension" title="Open FileSystem" onClick={(event) => { event.stopPropagation(); onPreviewEnd(connection.connectionInstanceId); onOpenFileSystem(connection.connectionInstanceId); }}><FolderOpen aria-hidden="true" size={15} /></button>
          <ConnectionActions connection={connection} moveTargets={groups.map(({ group, connections: members }): ConnectionGroupMoveTarget => ({ groupId: group.groupId, name: group.name, count: members.length, current: group.groupId === groupId, full: group.groupId !== UNGROUPED_GROUP_ID && members.length >= 10 && group.groupId !== groupId }))} onMoveToGroup={(targetGroupId) => { void onMoveInstance(connection.connectionInstanceId, targetGroupId, null, 'after'); }} onRename={() => onRename(connection.connectionInstanceId)} onAutomaticTitle={() => onAutomaticTitle(connection.connectionInstanceId)} onTerminate={() => onTerminate(connection.connectionInstanceId)} />
        </div>
      </article>
    );
  }
});
