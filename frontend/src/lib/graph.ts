import type { Edge, Node } from '@xyflow/react';
import type {
  KeyValueRow,
  NodeDefinition,
  NodeSettings,
  Params,
  VNodeData,
  WorkflowDocument,
  WorkflowSettings,
  WorkflowVariable,
} from '../types';

export const DEFAULT_NODE_SETTINGS: NodeSettings = {
  retry: { maxAttempts: 1, delayMs: 1000, backoff: false, maxDelayMs: 0 },
  timeoutSec: 0,
  onError: 'stop',
};

/** Variables the engine provides to every run. */
export const SYSTEM_VARIABLES = [
  { path: 'execution.id', description: 'ID выполнения' },
  { path: 'execution.startedAt', description: 'Время старта' },
  { path: 'execution.trigger', description: 'Источник запуска' },
  { path: 'workflow.id', description: 'ID Workflow' },
  { path: 'workflow.name', description: 'Название Workflow' },
];

export const RESERVED_KEYS = new Set(['execution', 'workflow', 'variables', 'env']);

export const SCHEMA_VERSION = 1;
export const NODE_TYPE = 'veylora';

export type FlowNode = Node<VNodeData, typeof NODE_TYPE>;

export function newId(prefix: string): string {
  const rnd =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID().slice(0, 8)
      : Math.random().toString(36).slice(2, 10);
  return `${prefix}-${rnd}`;
}

/** Converts a stored document into React Flow nodes and edges. */
export function fromDocument(doc: WorkflowDocument): { nodes: FlowNode[]; edges: Edge[] } {
  const nodes: FlowNode[] = (doc.nodes ?? []).map((n) => ({
    id: n.id,
    type: NODE_TYPE,
    position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
    data: { nodeType: n.type, name: n.name, key: n.key, params: n.params ?? {}, settings: n.settings },
  }));
  const edges: Edge[] = (doc.edges ?? []).map((e) => ({
    id: e.id,
    source: e.source,
    sourceHandle: e.sourceHandle,
    target: e.target,
    targetHandle: e.targetHandle,
  }));
  return { nodes, edges };
}

/** Converts React Flow state back into the persisted document. */
export function toDocument(
  nodes: FlowNode[],
  edges: Edge[],
  viewport: { x: number; y: number; zoom: number } | undefined,
  variables: WorkflowVariable[],
  settings: WorkflowSettings,
): WorkflowDocument {
  return {
    schemaVersion: SCHEMA_VERSION,
    variables,
    settings,
    nodes: nodes.map((n) => ({
      id: n.id,
      type: n.data.nodeType,
      name: n.data.name,
      key: n.data.key,
      params: n.data.params,
      settings: n.data.settings,
      position: { x: Math.round(n.position.x), y: Math.round(n.position.y) },
    })),
    edges: edges.map((e) => ({
      id: e.id,
      source: e.source,
      sourceHandle: e.sourceHandle ?? '',
      target: e.target,
      targetHandle: e.targetHandle ?? '',
    })),
    viewport,
  };
}

/** Default parameter values for a freshly added node. */
export function defaultParams(def: NodeDefinition): Params {
  const params: Params = {};
  for (const spec of def.params) {
    if (spec.kind === 'keyvalue') params[spec.key] = [];
    else if (spec.kind === 'boolean') params[spec.key] = Boolean(spec.default ?? false);
    else if (spec.default !== undefined && spec.default !== null) params[spec.key] = spec.default;
    else params[spec.key] = '';
  }
  return params;
}

/** Picks a variable name not used by any other node: http, http2, http3… */
export function uniqueKey(prefix: string, nodes: FlowNode[], exceptId?: string): string {
  const used = new Set(nodes.filter((n) => n.id !== exceptId).map((n) => n.data.key));
  const base = prefix.replace(/[^A-Za-z0-9_]/g, '_') || 'node';
  if (!used.has(base)) return base;
  for (let i = 2; ; i++) {
    if (!used.has(`${base}${i}`)) return `${base}${i}`;
  }
}

/** Picks a display name not used by another node: "Logger", "Logger 2"… */
export function uniqueName(base: string, nodes: FlowNode[]): string {
  const used = new Set(nodes.map((n) => n.data.name));
  if (!used.has(base)) return base;
  for (let i = 2; ; i++) {
    if (!used.has(`${base} ${i}`)) return `${base} ${i}`;
  }
}

/** True if adding source→target would create a cycle (target already reaches source). */
export function wouldCreateCycle(edges: Edge[], source: string, target: string): boolean {
  if (source === target) return true;
  const adj = new Map<string, string[]>();
  for (const e of edges) {
    const list = adj.get(e.source) ?? [];
    list.push(e.target);
    adj.set(e.source, list);
  }
  const stack = [target];
  const seen = new Set<string>();
  while (stack.length) {
    const id = stack.pop()!;
    if (id === source) return true;
    if (seen.has(id)) continue;
    seen.add(id);
    stack.push(...(adj.get(id) ?? []));
  }
  return false;
}

/** All nodes upstream of `id` — their outputs are available as variables. */
export function ancestors(edges: Edge[], id: string): Set<string> {
  const rev = new Map<string, string[]>();
  for (const e of edges) {
    const list = rev.get(e.target) ?? [];
    list.push(e.source);
    rev.set(e.target, list);
  }
  const out = new Set<string>();
  const stack = [...(rev.get(id) ?? [])];
  while (stack.length) {
    const cur = stack.pop()!;
    if (out.has(cur)) continue;
    out.add(cur);
    stack.push(...(rev.get(cur) ?? []));
  }
  return out;
}

export function keyValues(params: Params, key: string): KeyValueRow[] {
  const raw = params[key];
  if (!Array.isArray(raw)) return [];
  return raw.map((r) => {
    const row = (r ?? {}) as Record<string, unknown>;
    return {
      key: String(row.key ?? ''),
      value: String(row.value ?? ''),
      enabled: row.enabled === undefined ? true : Boolean(row.enabled),
    };
  });
}

export function str(params: Params, key: string, fallback = ''): string {
  const v = params[key];
  if (v === undefined || v === null) return fallback;
  return String(v);
}

export const KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;
