export type FileSystemErrorCode =
  | 'filesystem_unsupported'
  | 'filesystem_instance_not_found'
  | 'filesystem_no_transport'
  | 'filesystem_transport_unavailable'
  | 'filesystem_root_unavailable'
  | 'filesystem_root_changed'
  | 'filesystem_path_invalid'
  | 'filesystem_path_outside_root'
  | 'filesystem_not_found'
  | 'filesystem_permission_denied'
  | 'filesystem_listing_failed'
  | 'filesystem_filename_encoding'
  | 'filesystem_directory_too_large'
  | 'filesystem_content_too_large'
  | 'filesystem_content_unavailable'
  | 'filesystem_range_invalid'
  | 'filesystem_upload_conflict'
  | 'filesystem_upload_transport_unavailable'
  | 'filesystem_upload_failed'
  | 'filesystem_upload_cancelled'
  | 'filesystem_upload_not_found'
  | 'filesystem_protocol_error'
  | 'filesystem_timeout';

export type FileSystemError = Error & {
  code?: FileSystemErrorCode | string;
  status?: number;
  retryable?: boolean;
  root?: RootContext;
};

export type RootContext = {
  connectionInstanceId: string;
  absolutePath: string;
  relativePath: '.';
  source: 'tmux' | 'configured';
  status: 'current' | 'fallback';
  revision: string;
  resolvedAt: string;
};

export type FileEntry = {
  name: string;
  relativePath: string;
  absolutePath: string;
  type: 'directory' | 'file' | 'symlink' | 'other';
  size: number | null;
  modifiedAt: string | null;
  mode: number;
  symlink: boolean;
};

export type DirectoryResult = {
  connectionInstanceId: string;
  rootRevision: string;
  path: string;
  entries: FileEntry[];
  nextCursor: string | null;
};

export type FileMetadata = FileEntry & {
  mimeType: string;
  encoding: string;
  capabilities: { read: boolean; range: boolean; stream: boolean; download: boolean };
  consistencyToken: string;
};

export type UploadManifestFile = {
  part: string;
  relativePath: string;
  size: number;
  modifiedAt: string;
};

export type UploadManifest = {
  rootRevision: string;
  targetPath: string;
  conflictPolicy: 'refuse' | 'overwrite' | 'update-if-newer';
  files: UploadManifestFile[];
};

export type UploadFailure = { path: string; code: string; error?: string };
export type UploadStatus = {
  uploadId: string;
  status: 'queued' | 'running' | 'completed' | 'partial-failure' | 'failed' | 'cancelled';
  transport: 'pending' | 'rsync' | 'scp';
  targetPath: string;
  bytesSent: number;
  bytesTotal: number;
  currentPath: string;
  failures: UploadFailure[];
};

export type LocalUploadFile = { file: File; relativePath: string };
