import { useRef, useState } from 'react';
import { Modal } from '../ui/modal';
import type { LocalUploadFile } from './filesystem-types';

type Props = {
  target: { relativePath: string; absolutePath: string };
  onClose: () => void;
  onConfirm: (files: LocalUploadFile[], policy: 'refuse' | 'overwrite' | 'update-if-newer') => void;
};

export function UploadConfirmDialog({ target, onClose, onConfirm }: Props) {
  const fileInput = useRef<HTMLInputElement>(null);
  const folderInput = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<LocalUploadFile[]>([]);
  const [policy, setPolicy] = useState<'refuse' | 'overwrite' | 'update-if-newer'>('refuse');
  const choose = (event: React.ChangeEvent<HTMLInputElement>) => {
    const next = Array.from(event.currentTarget.files || []).map((file) => ({ file, relativePath: file.webkitRelativePath || file.name }));
    setFiles(next);
    event.currentTarget.value = '';
  };
  const total = files.reduce((sum, item) => sum + item.file.size, 0);
  return (
    <Modal onClose={onClose}>
      <div className="upload-confirm-dialog">
        <header><div><p className="eyebrow">REMOTE FILESYSTEM</p><h2>Upload to directory</h2></div></header>
        <div className="upload-target"><span>{target.relativePath}</span><small title={target.absolutePath}>{target.absolutePath}</small></div>
        {!files.length ? (
          <div className="upload-source-actions">
            <button className="secondary" type="button" onClick={() => fileInput.current?.click()}>Choose files</button>
            <button className="secondary" type="button" onClick={() => folderInput.current?.click()}>Choose folder</button>
            <input id="upload-files" name="files" ref={fileInput} type="file" multiple hidden onChange={choose} />
            <input id="upload-folder" name="folder" ref={folderInput} type="file" hidden onChange={choose} {...({ webkitdirectory: '', directory: '' } as React.InputHTMLAttributes<HTMLInputElement>)} />
            <p>Select local files or a folder. No data is sent before confirmation.</p>
          </div>
        ) : (
          <>
            <div className="upload-summary"><strong>{files.length} file{files.length === 1 ? '' : 's'}</strong><span>{formatSize(total)}</span><button className="text-button" type="button" onClick={() => setFiles([])}>Choose again</button></div>
            <ul className="upload-file-list">{files.slice(0, 8).map((item) => <li key={item.relativePath} title={item.relativePath}>{item.relativePath}</li>)}{files.length > 8 && <li>and {files.length - 8} more...</li>}</ul>
            <label>Conflict policy<select id="upload-conflict-policy" name="conflictPolicy" value={policy} onChange={(event) => setPolicy(event.target.value as typeof policy)}><option value="refuse">Do not overwrite</option><option value="overwrite">Overwrite existing files</option><option value="update-if-newer">Only update newer files</option></select></label>
          </>
        )}
        <footer><button className="text-button" type="button" onClick={onClose}>Cancel</button><button className="primary" type="button" disabled={!files.length} onClick={() => onConfirm(files, policy)}>Confirm upload</button></footer>
      </div>
    </Modal>
  );
}

function formatSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
