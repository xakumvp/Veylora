import { create } from 'zustand';
import {
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge,
  type EdgeChange,
  type NodeChange,
} from '@xyflow/react';
import { api, errorMessage } from '../services/api';
import { preparePaste, readClipboard, serializeSelection, writeClipboard } from '../lib/clipboard';
import { autoLayout } from '../lib/layout';
import {
  defaultParams,
  fromDocument,
  newId,
  NODE_TYPE,
  toDocument,
  uniqueKey,
  uniqueName,
  type FlowNode,
} from '../lib/graph';
import type {
  NodeDefinition,
  NodeSettings,
  Viewport,
  Workflow,
  WorkflowDocument,
  WorkflowSettings,
  WorkflowVariable,
} from '../types';
import { toast } from './ui';

const HISTORY_LIMIT = 100;
const COALESCE_MS = 1200;

interface Snapshot {
  nodes: FlowNode[];
  edges: Edge[];
}

export type SaveState = 'saved' | 'dirty' | 'saving' | 'error';

interface EditorState {
  workflowId: string | null;
  name: string;
  description: string;
  nodes: FlowNode[];
  edges: Edge[];
  viewport?: Viewport;
  variables: WorkflowVariable[];
  wfSettings: WorkflowSettings;
  past: Snapshot[];
  future: Snapshot[];
  revision: number;
  saveState: SaveState;
  saveError?: string;
  lastSavedAt?: number;

  load: (w: Workflow) => void;
  clear: () => void;
  setMeta: (name: string, description: string) => void;

  onNodesChange: (changes: NodeChange<FlowNode>[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;
  connect: (c: Connection) => void;
  beginDrag: () => void;
  setViewport: (v: Viewport) => void;

  addNode: (def: NodeDefinition, position: { x: number; y: number }) => string;
  renameNode: (id: string, name: string) => void;
  setNodeKey: (id: string, key: string) => void;
  setParam: (id: string, param: string, value: unknown) => void;
  setNodeSettings: (id: string, settings: NodeSettings | undefined) => void;
  setVariables: (variables: WorkflowVariable[]) => void;
  setWorkflowSettings: (settings: WorkflowSettings) => void;
  deleteNodes: (ids: string[]) => void;
  duplicateNode: (id: string) => void;
  selectOnly: (id: string | null) => void;
  selectAll: () => void;
  copySelection: () => Promise<number>;
  cutSelection: () => Promise<void>;
  paste: (at?: { x: number; y: number }) => Promise<number>;
  layout: () => void;

  undo: () => void;
  redo: () => void;
  save: () => Promise<boolean>;
  document: () => WorkflowDocument;
}

// Edits to the same field within COALESCE_MS share one undo step.
let lastEdit: { target: string; at: number } | null = null;
let lastRemovalSnapshotAt = 0;
let inflight: Promise<boolean> | null = null;

const clean = (nodes: FlowNode[]): FlowNode[] =>
  nodes.map((n) => ({ ...n, selected: false, dragging: false }));

export const useEditor = create<EditorState>((set, get) => {
  const pushHistory = () => {
    const { nodes, edges, past } = get();
    set({ past: [...past.slice(-(HISTORY_LIMIT - 1)), { nodes: clean(nodes), edges }], future: [] });
  };

  const changed = () => set((s) => ({ revision: s.revision + 1, saveState: 'dirty' }));

  const coalescedHistory = (target: string) => {
    const now = Date.now();
    if (!lastEdit || lastEdit.target !== target || now - lastEdit.at > COALESCE_MS) pushHistory();
    lastEdit = { target, at: now };
  };

  const updateData = (id: string, patch: Partial<FlowNode['data']>) =>
    set((s) => ({
      nodes: s.nodes.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...patch } } : n)),
    }));

  return {
    workflowId: null,
    name: '',
    description: '',
    nodes: [],
    edges: [],
    variables: [],
    wfSettings: { timeoutSec: 0 },
    past: [],
    future: [],
    revision: 0,
    saveState: 'saved',

    load: (w) => {
      const { nodes, edges } = fromDocument(w.document);
      lastEdit = null;
      set({
        workflowId: w.id,
        name: w.name,
        description: w.description,
        nodes,
        edges,
        viewport: w.document.viewport,
        variables: w.document.variables ?? [],
        wfSettings: w.document.settings ?? { timeoutSec: 0 },
        past: [],
        future: [],
        revision: 0,
        saveState: 'saved',
        saveError: undefined,
        lastSavedAt: w.updatedAt,
      });
    },

    clear: () =>
      set({
        workflowId: null,
        name: '',
        description: '',
        nodes: [],
        edges: [],
        past: [],
        future: [],
        saveState: 'saved',
      }),

    setMeta: (name, description) => set({ name, description }),

    onNodesChange: (changes) => {
      const removing = changes.some((c) => c.type === 'remove');
      if (removing && Date.now() - lastRemovalSnapshotAt > 50) {
        pushHistory();
        lastRemovalSnapshotAt = Date.now();
      }
      set((s) => ({ nodes: applyNodeChanges(changes, s.nodes) }));
      const dragEnded = changes.some((c) => c.type === 'position' && c.dragging === false);
      if (removing || dragEnded) changed();
    },

    onEdgesChange: (changes) => {
      const removing = changes.some((c) => c.type === 'remove');
      if (removing && Date.now() - lastRemovalSnapshotAt > 50) {
        pushHistory();
        lastRemovalSnapshotAt = Date.now();
      }
      set((s) => ({ edges: applyEdgeChanges(changes, s.edges) }));
      if (removing) changed();
    },

    connect: (c) => {
      if (!c.source || !c.target) return;
      pushHistory();
      const edge: Edge = {
        id: newId('e'),
        source: c.source,
        sourceHandle: c.sourceHandle ?? 'out',
        target: c.target,
        targetHandle: c.targetHandle ?? 'in',
      };
      set((s) => ({ edges: [...s.edges, edge] }));
      changed();
    },

    beginDrag: () => pushHistory(),

    setViewport: (viewport) => {
      set({ viewport });
      if (get().workflowId) changed();
    },

    addNode: (def, position) => {
      pushHistory();
      const { nodes } = get();
      const id = newId(def.keyPrefix);
      const node: FlowNode = {
        id,
        type: NODE_TYPE,
        position,
        selected: true,
        data: {
          nodeType: def.type,
          name: uniqueName(def.name, nodes),
          key: uniqueKey(def.keyPrefix, nodes),
          params: defaultParams(def),
        },
      };
      set({ nodes: [...nodes.map((n) => ({ ...n, selected: false })), node] });
      changed();
      return id;
    },

    renameNode: (id, name) => {
      coalescedHistory(`${id}:name`);
      updateData(id, { name });
      changed();
    },

    setNodeKey: (id, key) => {
      coalescedHistory(`${id}:key`);
      updateData(id, { key });
      changed();
    },

    setParam: (id, param, value) => {
      coalescedHistory(`${id}:${param}`);
      set((s) => ({
        nodes: s.nodes.map((n) =>
          n.id === id ? { ...n, data: { ...n.data, params: { ...n.data.params, [param]: value } } } : n,
        ),
      }));
      changed();
    },

    setNodeSettings: (id, settings) => {
      coalescedHistory(`${id}:settings`);
      updateData(id, { settings });
      changed();
    },

    setVariables: (variables) => {
      set({ variables });
      changed();
    },

    setWorkflowSettings: (wfSettings) => {
      set({ wfSettings });
      changed();
    },

    deleteNodes: (ids) => {
      if (!ids.length) return;
      pushHistory();
      const drop = new Set(ids);
      set((s) => ({
        nodes: s.nodes.filter((n) => !drop.has(n.id)),
        edges: s.edges.filter((e) => !drop.has(e.source) && !drop.has(e.target)),
      }));
      changed();
    },

    duplicateNode: (id) => {
      const src = get().nodes.find((n) => n.id === id);
      if (!src) return;
      pushHistory();
      const { nodes } = get();
      const prefix = src.data.key.replace(/\d+$/, '') || 'node';
      const copy: FlowNode = {
        id: newId(prefix),
        type: NODE_TYPE,
        position: { x: src.position.x + 40, y: src.position.y + 40 },
        selected: true,
        data: {
          ...src.data,
          params: structuredClone(src.data.params),
          name: uniqueName(src.data.name, nodes),
          key: uniqueKey(prefix, nodes),
        },
      };
      set({ nodes: [...nodes.map((n) => ({ ...n, selected: false })), copy] });
      changed();
    },

    selectOnly: (id) => set((s) => ({ nodes: s.nodes.map((n) => ({ ...n, selected: n.id === id })) })),

    selectAll: () => set((s) => ({ nodes: s.nodes.map((n) => ({ ...n, selected: true })) })),

    copySelection: async () => {
      const text = serializeSelection(get().nodes, get().edges);
      if (!text) return 0;
      await writeClipboard(text);
      return get().nodes.filter((n) => n.selected).length;
    },

    cutSelection: async () => {
      const ids = get()
        .nodes.filter((n) => n.selected)
        .map((n) => n.id);
      if (!ids.length) return;
      await get().copySelection();
      get().deleteNodes(ids);
    },

    paste: async (at) => {
      if (!get().workflowId) return 0;
      const payload = await readClipboard();
      if (!payload?.nodes.length) return 0;
      pushHistory();
      const { nodes, edges } = preparePaste(payload, get().nodes, at);
      set((s) => ({
        nodes: [...s.nodes.map((n) => ({ ...n, selected: false })), ...nodes],
        edges: [...s.edges, ...edges],
      }));
      changed();
      return nodes.length;
    },

    layout: () => {
      const { nodes, edges } = get();
      if (!nodes.length) return;
      pushHistory();
      const pos = autoLayout(nodes, edges);
      set({ nodes: nodes.map((n) => (pos.has(n.id) ? { ...n, position: pos.get(n.id)! } : n)) });
      changed();
    },

    undo: () => {
      const { past, future, nodes, edges } = get();
      const prev = past[past.length - 1];
      if (!prev) return;
      lastEdit = null;
      set({
        nodes: prev.nodes,
        edges: prev.edges,
        past: past.slice(0, -1),
        future: [...future, { nodes: clean(nodes), edges }],
      });
      changed();
    },

    redo: () => {
      const { past, future, nodes, edges } = get();
      const next = future[future.length - 1];
      if (!next) return;
      lastEdit = null;
      set({
        nodes: next.nodes,
        edges: next.edges,
        future: future.slice(0, -1),
        past: [...past, { nodes: clean(nodes), edges }],
      });
      changed();
    },

    save: async () => {
      // Serialise saves: if one is in flight, wait for it and then save again if still dirty.
      if (inflight) await inflight.catch(() => false);
      const { workflowId, saveState } = get();
      if (!workflowId || saveState === 'saved') return true;
      const revision = get().revision;
      set({ saveState: 'saving', saveError: undefined });
      inflight = (async () => {
        try {
          const res = await api.saveWorkflow(workflowId, get().document());
          if (res.warning) {
            toast('warning', 'Автоматизация отключена', res.warning);
            window.dispatchEvent(new CustomEvent('veylora:automation-changed', { detail: workflowId }));
          }
          if (get().workflowId !== workflowId) return true;
          set({ saveState: get().revision === revision ? 'saved' : 'dirty', lastSavedAt: res.updatedAt });
          return true;
        } catch (err) {
          if (get().workflowId === workflowId) set({ saveState: 'error', saveError: errorMessage(err) });
          return false;
        } finally {
          inflight = null;
        }
      })();
      return inflight;
    },

    document: () => {
      const { nodes, edges, viewport, variables, wfSettings } = get();
      return toDocument(nodes, edges, viewport, variables, wfSettings);
    },
  };
});
