import { useEffect, type Dispatch, type SetStateAction } from 'react';
import { appearanceFromStorageEvent, type TerminalAppearance } from './appearance-model';

export function useAppearanceStorage(setAppearance: Dispatch<SetStateAction<TerminalAppearance>>): void {
  useEffect(() => {
    const handler = (event: StorageEvent) => {
      const next = appearanceFromStorageEvent(event);
      if (next) setAppearance(next);
    };
    window.addEventListener('storage', handler);
    return () => window.removeEventListener('storage', handler);
  }, [setAppearance]);
}
