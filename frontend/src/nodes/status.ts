import type { NodeStatus } from '../types';

export const statusMeta: Record<NodeStatus, { label: string; color: string; text: string }> = {
  IDLE: { label: 'Idle', color: 'bg-faint', text: 'text-faint' },
  WAITING: { label: 'Waiting', color: 'bg-muted', text: 'text-muted' },
  RUNNING: { label: 'Running', color: 'bg-accent', text: 'text-accent' },
  SUCCESS: { label: 'Success', color: 'bg-success', text: 'text-success' },
  FAILED: { label: 'Failed', color: 'bg-error', text: 'text-error' },
  SKIPPED: { label: 'Skipped', color: 'bg-faint', text: 'text-faint' },
  CANCELLED: { label: 'Cancelled', color: 'bg-warning', text: 'text-warning' },
  TIMEOUT: { label: 'Timeout', color: 'bg-error', text: 'text-error' },
};

export const levelMeta: Record<string, string> = {
  DEBUG: 'text-faint',
  INFO: 'text-info',
  WARNING: 'text-warning',
  ERROR: 'text-error',
};
