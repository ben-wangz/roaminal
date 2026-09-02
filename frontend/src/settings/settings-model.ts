export type SettingsSection = 'definitions' | 'keys' | 'interface' | 'notifications';

export const DEFAULT_SETTINGS_SECTION: SettingsSection = 'definitions';

export const SETTINGS_SECTIONS: ReadonlyArray<{
  id: SettingsSection;
  label: string;
}> = [
  { id: 'definitions', label: 'Connection definitions' },
  { id: 'keys', label: 'SSH keys' },
  { id: 'interface', label: 'Interface' },
  { id: 'notifications', label: 'Notifications' },
];
