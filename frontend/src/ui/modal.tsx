import type { ReactNode } from 'react';
import { Dialog } from '@base-ui/react/dialog';

export function Modal({ children, onClose }: { children: ReactNode; onClose?: () => void }) {
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open) onClose?.();
      }}
    >
      <Dialog.Portal>
        <Dialog.Backdrop className="modal-backdrop" />
        <Dialog.Popup className="modal-panel" aria-modal="true">
          {children}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
