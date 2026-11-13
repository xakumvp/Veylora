import { create } from 'zustand';

export type ToastKind = 'success' | 'error' | 'warning' | 'info';

export interface Toast {
  id: number;
  kind: ToastKind;
  title: string;
  message?: string;
}

interface PromptRequest {
  title: string;
  label?: string;
  initial?: string;
  confirmLabel?: string;
  resolve: (value: string | null) => void;
}

interface ConfirmRequest {
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  resolve: (ok: boolean) => void;
}

export interface ContextMenuState {
  x: number;
  y: number;
  kind: 'node' | 'pane';
  nodeId?: string;
  flow?: { x: number; y: number };
}

interface UIState {
  toasts: Toast[];
  prompt: PromptRequest | null;
  confirm: ConfirmRequest | null;
  settingsOpen: boolean;
  aboutOpen: boolean;
  executionPanelOpen: boolean;
  contextMenu: ContextMenuState | null;
  paletteOpen: boolean;
  versionsOpen: boolean;
  /** Project for which the New Workflow dialog is open. */
  newWorkflowFor: string | null;

  toast: (kind: ToastKind, title: string, message?: string) => void;
  dismissToast: (id: number) => void;
  askText: (opts: Omit<PromptRequest, 'resolve'>) => Promise<string | null>;
  askConfirm: (opts: Omit<ConfirmRequest, 'resolve'>) => Promise<boolean>;
  closePrompt: (value: string | null) => void;
  closeConfirm: (ok: boolean) => void;
  setSettingsOpen: (open: boolean) => void;
  setAboutOpen: (open: boolean) => void;
  setExecutionPanelOpen: (open: boolean) => void;
  openContextMenu: (m: ContextMenuState | null) => void;
  setPaletteOpen: (open: boolean) => void;
  setVersionsOpen: (open: boolean) => void;
  setNewWorkflowFor: (projectId: string | null) => void;
}

let toastSeq = 0;

export const useUI = create<UIState>((set, get) => ({
  toasts: [],
  prompt: null,
  confirm: null,
  settingsOpen: false,
  aboutOpen: false,
  executionPanelOpen: true,
  contextMenu: null,
  paletteOpen: false,
  versionsOpen: false,
  newWorkflowFor: null,

  toast: (kind, title, message) => {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts.slice(-4), { id, kind, title, message }] }));
    const ttl = kind === 'error' ? 8000 : kind === 'warning' ? 7000 : 3500;
    window.setTimeout(() => get().dismissToast(id), ttl);
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),

  askText: (opts) =>
    new Promise((resolve) => {
      get().prompt?.resolve(null);
      set({ prompt: { ...opts, resolve } });
    }),
  askConfirm: (opts) =>
    new Promise((resolve) => {
      get().confirm?.resolve(false);
      set({ confirm: { ...opts, resolve } });
    }),
  closePrompt: (value) => {
    get().prompt?.resolve(value);
    set({ prompt: null });
  },
  closeConfirm: (ok) => {
    get().confirm?.resolve(ok);
    set({ confirm: null });
  },
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
  setAboutOpen: (aboutOpen) => set({ aboutOpen }),
  setExecutionPanelOpen: (executionPanelOpen) => set({ executionPanelOpen }),
  openContextMenu: (contextMenu) => set({ contextMenu }),
  setPaletteOpen: (paletteOpen) => set({ paletteOpen, contextMenu: null }),
  setVersionsOpen: (versionsOpen) => set({ versionsOpen }),
  setNewWorkflowFor: (newWorkflowFor) => set({ newWorkflowFor }),
}));

export const toast = (kind: ToastKind, title: string, message?: string) =>
  useUI.getState().toast(kind, title, message);
