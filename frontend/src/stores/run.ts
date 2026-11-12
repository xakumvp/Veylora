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
}

interface RunState {
  /** Run currently executing (only one at a time). */
  activeRunId: string | null;
  /** Run whose logs/statuses are displayed (live or from history). */
  shownRunId: string | null;
  runWorkflowId: string | null;
  status: NodeStatus;
  startedAt?: number;
  durationMs?: number;
  error?: string;
  nodes: Record<string, NodeRunInfo>;
  logs: LogEntry[];
  executions: Execution[];
  issues: Issue[];
  starting: boolean;

  start: () => Promise<void>;
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

export const useRun = create<RunState>((set, get) => ({
  activeRunId: null,
  shownRunId: null,
  runWorkflowId: null,
  status: 'IDLE',
  nodes: {},
  logs: [],
  executions: [],
  issues: [],
  starting: false,

  start: async () => {
    const editor = useEditor.getState();
    if (!editor.workflowId || get().activeRunId || get().starting) return;
    const workflowId = editor.workflowId;
    set({ starting: true });
    try {
      void editor.save();
      const doc = editor.document();
      const runId = await api.runWorkflow(workflowId, doc);
      // run:started usually arrives first; make sure the state is consistent either way.
      if (get().activeRunId !== runId) {
        set({ activeRunId: runId, shownRunId: runId, runWorkflowId: workflowId, status: 'RUNNING' });
      }
    } catch (err) {
      const msg = errorMessage(err);
      toast('error', 'Workflow не запущен', msg);
    } finally {
      set({ starting: false });
    }
  },

  stop: async () => {
    try {
      await api.stopWorkflow();
    } catch (err) {
      toast('error', 'Не удалось остановить', errorMessage(err));
    }
  },

  onRunStarted: (ev) => {
    const current = useEditor.getState().workflowId;
    if (ev.workflowId !== current) {
      set({ activeRunId: ev.runId });
      return;
    }
    set({
      activeRunId: ev.runId,
      shownRunId: ev.runId,
      runWorkflowId: ev.workflowId,
      status: 'RUNNING',
      startedAt: ev.startedAt,
      durationMs: undefined,
      error: undefined,
      nodes: {},
      logs: [],
    });
  },

  onRunFinished: (ev) => {
    set({ activeRunId: null });
    if (ev.runId !== get().shownRunId) return;
    set({ status: ev.status, durationMs: ev.durationMs, error: ev.error });
    if (ev.status === 'SUCCESS') toast('success', 'Workflow выполнен', `за ${formatDuration(ev.durationMs)}`);
    else if (ev.status === 'FAILED') toast('error', 'Workflow завершён с ошибкой', ev.error);
    else if (ev.status === 'CANCELLED') toast('warning', 'Выполнение остановлено');
    if (ev.workflowId) void get().loadHistory(ev.workflowId);
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
    const s = get();
    const live = s.activeRunId && s.runWorkflowId === workflowId;
    if (live) return;
    set({
      shownRunId: null,
      runWorkflowId: workflowId,
      status: 'IDLE',
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
      if (useEditor.getState().workflowId === workflowId) set({ executions });
    } catch (err) {
      toast('error', 'История не загружена', errorMessage(err));
    }
  },

  showExecution: async (ex) => {
    if (get().activeRunId === ex.id) {
      set({ shownRunId: ex.id });
      return;
    }
    try {
      const logs = await api.executionLogs(ex.id);
      set({
        shownRunId: ex.id,
        status: ex.status,
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
    const workflowId = useEditor.getState().workflowId;
    if (!workflowId) return;
    if (get().activeRunId) {
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
      const id = await api.activeRun();
      set({ activeRunId: id || null });
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
