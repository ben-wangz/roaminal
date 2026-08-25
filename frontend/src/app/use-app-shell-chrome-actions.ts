import { useCallback, useEffect, type Dispatch, type SetStateAction } from 'react';
import { matchesShortcut, SHORTCUTS } from '../input/shortcuts';

type Params = {
  setSidebarOpen: Dispatch<SetStateAction<boolean>>;
  setVirtualKeyboardOpen: Dispatch<SetStateAction<boolean>>;
  setPreviewConnectionInstanceId: Dispatch<SetStateAction<string | null>>;
};

export function useAppShellChromeActions({
  setSidebarOpen,
  setVirtualKeyboardOpen,
  setPreviewConnectionInstanceId,
}: Params) {
  const toggleSidebar = useCallback(() => {
    setSidebarOpen((value) => {
      if (value) setPreviewConnectionInstanceId(null);
      return !value;
    });
    setVirtualKeyboardOpen(false);
  }, [setPreviewConnectionInstanceId, setSidebarOpen, setVirtualKeyboardOpen]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (!matchesShortcut(event, SHORTCUTS[2])) return;
      event.preventDefault();
      toggleSidebar();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [toggleSidebar]);

  return { toggleSidebar };
}
