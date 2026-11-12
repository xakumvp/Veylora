import { useEffect } from 'react';
import { api } from '../services/api';
import { subscribeEngine } from '../services/events';
import { useEditor } from '../stores/editor';
import { useRun } from '../stores/run';
import { useUI } from '../stores/ui';

const AUTOSAVE_MS = 800;
const VALIDATE_MS = 350;

/** Forwards Go engine events into the run store. */
export function useEngineEvents() {
  useEffect(() => {
    const run = useRun.getState();
    return subscribeEngine({
      onRunStarted: run.onRunStarted,
      onRunFinished: run.onRunFinished,
      onNodeStatus: run.onNodeStatus,
      onLog: run.onLog,
    });
  }, []);
}

/** Saves the workflow shortly after every change (positions, params, connections). */
export function useAutosave() {
  const revision = useEditor((s) => s.revision);
  const saveState = useEditor((s) => s.saveState);
  useEffect(() => {
    if (saveState !== 'dirty') return;
    const t = window.setTimeout(() => void useEditor.getState().save(), AUTOSAVE_MS);
    return () => window.clearTimeout(t);
  }, [revision, saveState]);

  // Last-chance save when the window is closing.
  useEffect(() => {
    const onUnload = () => {
      if (useEditor.getState().saveState !== 'saved') void useEditor.getState().save();
    };
    window.addEventListener('beforeunload', onUnload);
    return () => window.removeEventListener('beforeunload', onUnload);
  }, []);
}

/** Re-validates the graph in Go after edits so the Inspector and nodes show problems live. */
export function useLiveValidation() {
  const revision = useEditor((s) => s.revision);
  const workflowId = useEditor((s) => s.workflowId);
  useEffect(() => {
    if (!workflowId) {
      useRun.getState().setIssues([]);
      return;
    }
    let cancelled = false;
    const t = window.setTimeout(async () => {
      try {
        const issues = await api.validateWorkflow(useEditor.getState().document());
        if (!cancelled) useRun.getState().setIssues(issues ?? []);
      } catch {
        /* validation is advisory */
      }
    }, VALIDATE_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
  }, [revision, workflowId]);
}

function isTyping(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el) return false;
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName);
}

/** Global shortcuts: Ctrl+S, Ctrl+Z, Ctrl+Shift+Z / Ctrl+Y, Ctrl+Enter, Esc. */
export function useShortcuts() {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      const ui = useUI.getState();
      if (ui.prompt || ui.confirm || ui.settingsOpen || ui.aboutOpen) return;
      const key = e.key.toLowerCase();
      if (mod && key === 's') {
        e.preventDefault();
        void useEditor.getState().save();
      } else if (mod && key === 'enter') {
        e.preventDefault();
        void useRun.getState().start();
      } else if (mod && !isTyping(e.target) && ((key === 'z' && e.shiftKey) || key === 'y')) {
        e.preventDefault();
        useEditor.getState().redo();
      } else if (mod && !isTyping(e.target) && key === 'z') {
        e.preventDefault();
        useEditor.getState().undo();
      } else if (mod && !isTyping(e.target) && key === 'd') {
        e.preventDefault();
        const sel = useEditor.getState().nodes.find((n) => n.selected);
        if (sel) useEditor.getState().duplicateNode(sel.id);
      } else if (key === 'escape' && !isTyping(e.target)) {
        useEditor.getState().selectOnly(null);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
}
