import { ArrowLeft, FileQuestion, LoaderCircle } from 'lucide-react';
import type { FileEntry, RootContext } from './filesystem-types';
import { FilePreview } from './file-preview';

type Props = {
  instanceId: string;
  root: RootContext | null;
  entry: FileEntry | null;
  onBackToTerminal: () => void;
  onToast: (message: string, kind?: 'info' | 'success' | 'error') => void;
  onRootChanged: () => void;
};

export function FilePreviewWorkspace({ instanceId, root, entry, onBackToTerminal, onToast, onRootChanged }: Props) {
  if (!root) {
    return <div className="file-preview-workspace"><div className="filesystem-preview-empty" role="region" aria-label="File preview"><LoaderCircle className="spin" size={22} aria-hidden="true" /><span>Loading file preview</span><button autoFocus className="icon-button file-preview-back-terminal" type="button" onClick={onBackToTerminal} aria-label="Back to Terminal" title="Back to Terminal" data-testid="file-preview-back-terminal"><ArrowLeft size={16} aria-hidden="true" /></button></div></div>;
  }
  if (!entry) {
    return <div className="file-preview-workspace"><div className="filesystem-preview-empty" role="region" aria-label="File preview"><FileQuestion size={25} aria-hidden="true" /><span>File preview is unavailable.</span><button autoFocus className="icon-button file-preview-back-terminal" type="button" onClick={onBackToTerminal} aria-label="Back to Terminal" title="Back to Terminal" data-testid="file-preview-back-terminal"><ArrowLeft size={16} aria-hidden="true" /></button></div></div>;
  }
  return (
    <div className="file-preview-workspace">
      <FilePreview instanceId={instanceId} root={root} entry={entry} onBackToTerminal={onBackToTerminal} onToast={onToast} onRootChanged={onRootChanged} />
    </div>
  );
}
