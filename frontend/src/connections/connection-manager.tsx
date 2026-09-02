import { SettingsPage } from './connection-manager-settings';
import { useConnectionManagerController, type ConnectionManagerProps } from './connection-manager-controller';

export type { ConnectionManagerProps } from './connection-manager-controller';

export function ConnectionManager(props: ConnectionManagerProps) {
  const controller = useConnectionManagerController(props);
  return <SettingsPage {...props} controller={controller} />;
}
