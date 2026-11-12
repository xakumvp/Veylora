import { create } from 'zustand';
import { api, errorMessage } from '../services/api';
import type { Execution, Issue, LogEntry, NodeEvent, NodeStatus, RunEvent } from '../types';
import { useEditor } from './editor';
import { toast } from './ui';

const MAX_LIVE_LOGS = 2000;

export interface NodeRunInfo {
  status: NodeStatus;
  error?: string;
  output?: unknown;
  branch?: string;
  durationMs?: number;
  attempts?: number;
  handled?: boolean;
}

interface RunState {
  /** Runs in progress: workflowId → runId (several workflows may run at once). */
  activeRuns: Record<string, string>;
  /** Run whose logs/statuses are displayed (live or from history). */
  shownRunId: string | null;
  runWorkflowId: string | null;
  status: NodeStatus;
  trigger?: string;
  startedAt?: number;
  durationMs?: number;
  error?: string;
  nodes: Record<string, NodeRunInfo>;
  logs: LogEntry[];
  executions: Execution[];
  issues: Issue[];
  starting: boolean;

  start: (triggerNodeId?: string) => Promise<void>;
  stop: () => Promise<void>;
  onRunStarted: (ev: RunEvent) => void;
  onRunFinished: (ev: RunEvent) => void;
  onNodeStatus: (ev: NodeEvent) => void;
  onLog: (l: LogEntry) => void;
  resetForWorkflow: (workflowId: string) => void;
  loadHistory: (workflowId: string) => Promise<void>;
  showExecution: (ex: Execution) => Promise<void>;
  clearHistory: () => Promise<void>;
  setIssues: (issues: Issue[]) => void;
  syncActive: () => Promise<void>;
}

const currentWorkflow = () => useEditor.getState().workflowId;

/** Trigger node selected on the canvas, if any — Run starts from it. */
function selectedTrigger(isTrigger: (type: string) => boolean): string {
  const sel = useEditor.getState().nodes.filter((n) => n.selected);
  return sel.length === 1 && isTrigger(sel[0].data.nodeType) ? sel[0].id : '';
}

let triggerCheck: (type: string) => boolean = () => false;
export const setTriggerCheck = (fn: (type: string) => boolean) => {
  triggerCheck = fn;
};

export const useRun = create<RunState>((set, get) => ({
  activeRuns: {},
  shownRunId: null,
  runWorkflowId: null,
  status: 'IDLE',
  nodes: {},
  logs: [],
  executions: [],
  issues: [],
  starting: false,

  start: async (triggerNodeId) => {
    const editor = useEditor.getState();
    const workflowId = editor.workflowId;
    if (!workflowId || get().activeRuns[workflowId] || get().starting) return;
    set({ starting: true });
    try {
      void editor.save();
      const runId = await api.runWorkflow(
        workflowId,
        editor.document(),
        triggerNodeId ?? selectedTrigger(triggerCheck),
      );
      if (!get().activeRuns[workflowId]) {
        set((s) => ({
          activeRuns: { ...s.activeRuns, [workflowId]: runId },
          shownRunId: runId,
          runWorkflowId: workflowId,
          status: 'RUNNING',
        }));
      }
    } catch (err) {
      toast('error', 'Workflow не запущен', errorMessage(err));
    } finally {
      set({ starting: false });
    }
  },

  stop: async () => {
    const workflowId = currentWorkflow();
    if (!workflowId) return;
    try {
      await api.stopWorkflow(workflowId);
    } catch (err) {
      toast('error', 'Не удалось остановить', errorMessage(err));
    }
  },

  onRunStarted: (ev) => {
    set((s) => ({ activeRuns: { ...s.activeRuns, [ev.workflowId]: ev.runId } }));
    if (ev.workflowId !== currentWorkflow()) return;
    set({
      shownRunId: ev.runId,
      runWorkflowId: ev.workflowId,
      status: 'RUNNING',
      trigger: ev.trigger,
      startedAt: ev.startedAt,
      durationMs: undefined,
      error: undefined,
      nodes: {},
      logs: [],
    });
  },

  onRunFinished: (ev) => {
    set((s) => {
      const activeRuns = { ...s.activeRuns };
      if (activeRuns[ev.workflowId] === ev.runId) delete activeRuns[ev.workflowId];
      return { activeRuns };
    });
    if (ev.trigger && ev.trigger !== 'manual') {
      // Automatic run: next fire times changed.
      window.dispatchEvent(new CustomEvent('veylora:automation-changed', { detail: ev.workflowId }));
    }
    if (ev.workflowId !== currentWorkflow()) return;
    void get().loadHistory(ev.workflowId);
    if (ev.runId !== get().shownRunId) return;
    set({ status: ev.status, durationMs: ev.durationMs, error: ev.error });
    const auto = ev.trigger && ev.trigger !== 'manual';
    if (ev.status === 'SUCCESS' && !auto)
      toast('success', 'Workflow выполнен', `за ${formatDuration(ev.durationMs)}`);
    else if (ev.status === 'FAILED') toast('error', 'Workflow завершён с ошибкой', ev.error);
    else if (ev.status === 'TIMEOUT') toast('error', 'Превышен таймаут Workflow', ev.error);
    else if (ev.status === 'CANCELLED' && !auto) toast('warning', 'Выполнение остановлено');
  },

  onNodeStatus: (ev) => {
    if (ev.runId !== get().shownRunId) return;
    set((s) => ({
      nodes: {
        ...s.nodes,
        [ev.nodeId]: {
          status: ev.status,
          error: ev.error,
          output: ev.output ?? s.nodes[ev.nodeId]?.output,
          branch: ev.branch,
          durationMs: ev.durationMs,
          attempts: ev.attempts,
          handled: ev.handled,
        },
      },
    }));
  },

  onLog: (l) => {
    if (l.runId !== get().shownRunId) return;
    set((s) => ({
      logs: s.logs.length >= MAX_LIVE_LOGS ? [...s.logs.slice(-MAX_LIVE_LOGS + 1), l] : [...s.logs, l],
    }));
  },

  resetForWorkflow: (workflowId) => {
    const live = get().activeRuns[workflowId];
    set({
      shownRunId: live ?? null,
      runWorkflowId: workflowId,
      status: live ? 'RUNNING' : 'IDLE',
      trigger: undefined,
      nodes: {},
      logs: [],
      error: undefined,
      durationMs: undefined,
      issues: [],
      executions: [],
    });
  },

  loadHistory: async (workflowId) => {
    try {
      const executions = await api.listExecutions(workflowId);
      if (currentWorkflow() === workflowId) set({ executions });
    } catch (err) {
      toast('error', 'История не загружена', errorMessage(err));
    }
  },

  showExecution: async (ex) => {
    if (get().activeRuns[ex.workflowId] === ex.id) {
      set({ shownRunId: ex.id });
      return;
    }
    try {
      const logs = await api.executionLogs(ex.id);
      set({
        shownRunId: ex.id,
        status: ex.status,
        trigger: ex.trigger,
        startedAt: ex.startedAt,
        durationMs: ex.durationMs,
        error: ex.error,
        logs,
        nodes: {},
      });
    } catch (err) {
      toast('error', 'Логи не загружены', errorMessage(err));
    }
  },

  clearHistory: async () => {
    const workflowId = currentWorkflow();
    if (!workflowId) return;
    if (get().activeRuns[workflowId]) {
      toast('warning', 'Дождитесь окончания выполнения');
      return;
    }
    try {
      await api.clearExecutions(workflowId);
      set({
        executions: [],
        logs: [],
        nodes: {},
        status: 'IDLE',
        shownRunId: null,
        error: undefined,
        durationMs: undefined,
      });
    } catch (err) {
      toast('error', 'Не удалось очистить', errorMessage(err));
    }
  },

  setIssues: (issues) => set({ issues }),

  syncActive: async () => {
    try {
      const runs = await api.activeRuns();
      set({ activeRuns: Object.fromEntries((runs ?? []).map((r) => [r.workflowId, r.runId])) });
    } catch {
      /* backend not ready */
    }
  },
}));

export function formatDuration(ms?: number): string {
  if (ms === undefined || ms === null) return '—';
  if (ms < 1000) return `${ms} мс`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 2 : 1)} с`;
  const m = Math.floor(ms / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  return `${m} мин ${s} с`;
}

export const triggerLabels: Record<string, string> = {
  manual: 'вручную',
  schedule: 'расписание',
  file: 'файл',
  startup: 'старт',
};
