import { create } from 'zustand';
import { api, errorMessage } from '../services/api';
import type { AppInfo, NodeDefinition, Project, Settings, WorkflowMeta } from '../types';
import { useEditor } from './editor';
import { useRun } from './run';
import { toast } from './ui';

const LAST_WORKFLOW_KEY = 'veylora:lastWorkflow';

function remember(id: string | null) {
  try {
    if (id) localStorage.setItem(LAST_WORKFLOW_KEY, id);
    else localStorage.removeItem(LAST_WORKFLOW_KEY);
  } catch {
    /* storage unavailable */
  }
}

function recall(): string | null {
  try {
    return localStorage.getItem(LAST_WORKFLOW_KEY);
  } catch {
    return null;
  }
}

let initStarted = false;

interface WorkspaceState {
  ready: boolean;
  fatalError?: string;
  info?: AppInfo;
  settings?: Settings;
  definitions: NodeDefinition[];
  defByType: Record<string, NodeDefinition>;
  projects: Project[];
  workflows: Record<string, WorkflowMeta[]>;
  expanded: Record<string, boolean>;
  activeProjectId: string | null;
  activeWorkflowId: string | null;
  opening: boolean;

  init: () => Promise<void>;
  toggleProject: (id: string) => void;
  refreshWorkflows: (projectId: string) => Promise<void>;
  createProject: (name: string) => Promise<void>;
  renameProject: (id: string, name: string) => Promise<void>;
  deleteProject: (id: string) => Promise<void>;
  createWorkflow: (projectId: string, name: string) => Promise<void>;
  openWorkflow: (id: string) => Promise<void>;
  renameWorkflow: (id: string, name: string, description?: string) => Promise<void>;
  deleteWorkflow: (id: string) => Promise<void>;
  importWorkflow: (projectId: string) => Promise<void>;
  exportWorkflow: () => Promise<void>;
  saveSettings: (s: Settings) => Promise<void>;
}

export const useWorkspace = create<WorkspaceState>((set, get) => ({
  ready: false,
  definitions: [],
  defByType: {},
  projects: [],
  workflows: {},
  expanded: {},
  activeProjectId: null,
  activeWorkflowId: null,
  opening: false,

  init: async () => {
    if (initStarted) return;
    initStarted = true;
    try {
      const [definitions, projects, info, settings] = await Promise.all([
        api.nodeDefinitions(),
        api.listProjects(),
        api.appInfo(),
        api.getSettings(),
      ]);
      const defByType = Object.fromEntries(definitions.map((d) => [d.type, d]));
      const lists = await Promise.all(projects.map((p) => api.listWorkflows(p.id)));
      const workflows = Object.fromEntries(projects.map((p, i) => [p.id, lists[i]]));
      set({ definitions, defByType, projects, workflows, info, settings, ready: true });
      void useRun.getState().syncActive();

      const all = Object.values(workflows).flat();
      const last = recall();
      const target = all.find((w) => w.id === last) ?? all[0];
      if (target) await get().openWorkflow(target.id);
      else if (projects[0]) set({ activeProjectId: projects[0].id, expanded: { [projects[0].id]: true } });
    } catch (err) {
      set({ fatalError: errorMessage(err), ready: true });
    }
  },

  toggleProject: (id) =>
    set((s) => ({ expanded: { ...s.expanded, [id]: !s.expanded[id] }, activeProjectId: id })),

  refreshWorkflows: async (projectId) => {
    const list = await api.listWorkflows(projectId);
    set((s) => ({ workflows: { ...s.workflows, [projectId]: list } }));
  },

  createProject: async (name) => {
    try {
      const p = await api.createProject(name);
      set((s) => ({
        projects: [...s.projects, p],
        workflows: { ...s.workflows, [p.id]: [] },
        expanded: { ...s.expanded, [p.id]: true },
        activeProjectId: p.id,
      }));
      toast('success', 'Проект создан', p.name);
    } catch (err) {
      toast('error', 'Проект не создан', errorMessage(err));
    }
  },

  renameProject: async (id, name) => {
    try {
      await api.renameProject(id, name);
      set((s) => ({
        projects: s.projects.map((p) => (p.id === id ? { ...p, name: name.trim() || p.name } : p)),
      }));
    } catch (err) {
      toast('error', 'Не удалось переименовать', errorMessage(err));
    }
  },

  deleteProject: async (id) => {
    try {
      await api.deleteProject(id);
      const wasActive = (get().workflows[id] ?? []).some((w) => w.id === get().activeWorkflowId);
      set((s) => {
        const workflows = { ...s.workflows };
        delete workflows[id];
        return {
          projects: s.projects.filter((p) => p.id !== id),
          workflows,
          activeProjectId: s.activeProjectId === id ? null : s.activeProjectId,
        };
      });
      if (wasActive) {
        useEditor.getState().clear();
        set({ activeWorkflowId: null });
        remember(null);
      }
      toast('success', 'Проект удалён');
    } catch (err) {
      toast('error', 'Не удалось удалить', errorMessage(err));
    }
  },

  createWorkflow: async (projectId, name) => {
    try {
      const w = await api.createWorkflow(projectId, name);
      await get().refreshWorkflows(projectId);
      set((s) => ({ expanded: { ...s.expanded, [projectId]: true } }));
      await get().openWorkflow(w.id);
    } catch (err) {
      toast('error', 'Workflow не создан', errorMessage(err));
    }
  },

  openWorkflow: async (id) => {
    const editor = useEditor.getState();
    if (editor.workflowId === id) return;
    set({ opening: true });
    try {
      if (editor.workflowId && editor.saveState !== 'saved') await editor.save();
      const w = await api.getWorkflow(id);
      useEditor.getState().load(w);
      useRun.getState().resetForWorkflow(w.id);
      set((s) => ({
        activeWorkflowId: w.id,
        activeProjectId: w.projectId,
        expanded: { ...s.expanded, [w.projectId]: true },
      }));
      remember(w.id);
      void useRun.getState().loadHistory(w.id);
    } catch (err) {
      toast('error', 'Workflow не открыт', errorMessage(err));
    } finally {
      set({ opening: false });
    }
  },

  renameWorkflow: async (id, name, description) => {
    const meta = Object.values(get().workflows)
      .flat()
      .find((w) => w.id === id);
    if (!meta) return;
    const desc = description ?? meta.description;
    try {
      await api.renameWorkflow(id, name, desc);
      const clean = name.trim() || meta.name;
      set((s) => ({
        workflows: {
          ...s.workflows,
          [meta.projectId]: (s.workflows[meta.projectId] ?? []).map((w) =>
            w.id === id ? { ...w, name: clean, description: desc } : w,
          ),
        },
      }));
      if (useEditor.getState().workflowId === id) useEditor.getState().setMeta(clean, desc);
    } catch (err) {
      toast('error', 'Не удалось переименовать', errorMessage(err));
    }
  },

  deleteWorkflow: async (id) => {
    const meta = Object.values(get().workflows)
      .flat()
      .find((w) => w.id === id);
    if (!meta) return;
    if (useRun.getState().activeRunId && useRun.getState().runWorkflowId === id) {
      toast('warning', 'Сначала остановите выполнение');
      return;
    }
    try {
      await api.deleteWorkflow(id);
      await get().refreshWorkflows(meta.projectId);
      if (get().activeWorkflowId === id) {
        useEditor.getState().clear();
        set({ activeWorkflowId: null });
        remember(null);
      }
      toast('success', 'Workflow удалён');
    } catch (err) {
      toast('error', 'Не удалось удалить', errorMessage(err));
    }
  },

  importWorkflow: async (projectId) => {
    try {
      const res = await api.importWorkflow(projectId);
      if (!res) return;
      await get().refreshWorkflows(projectId);
      await get().openWorkflow(res.workflow.id);
      toast('success', 'Workflow импортирован', res.workflow.name);
      for (const w of res.warnings ?? []) toast('warning', 'Требуется внимание', w);
    } catch (err) {
      toast('error', 'Импорт не выполнен', errorMessage(err));
    }
  },

  exportWorkflow: async () => {
    const editor = useEditor.getState();
    if (!editor.workflowId) return;
    try {
      if (!(await editor.save())) throw new Error(useEditor.getState().saveError ?? 'не удалось сохранить');
      const res = await api.exportWorkflow(editor.workflowId);
      if (!res) return;
      toast('success', 'Workflow экспортирован', res.path);
      if (res.removedSecrets?.length) {
        toast(
          'warning',
          `Секреты не экспортированы (${res.removedSecrets.length})`,
          res.removedSecrets.join('; '),
        );
      }
    } catch (err) {
      toast('error', 'Экспорт не выполнен', errorMessage(err));
    }
  },

  saveSettings: async (s) => {
    try {
      const settings = await api.saveSettings(s);
      set({ settings });
      toast('success', 'Настройки сохранены');
    } catch (err) {
      toast('error', 'Настройки не сохранены', errorMessage(err));
    }
  },
}));
