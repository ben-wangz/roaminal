export type ToastKind = 'info' | 'success' | 'error';
export type ToastState = { message: string; kind: ToastKind };

export function Toast({ toast }: { toast: ToastState | null }) {
  return toast ? (
    <div className="toast" data-kind={toast.kind} role="status">
      {toast.message}
    </div>
  ) : null;
}
