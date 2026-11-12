import type { Edge } from '@xyflow/react';
import { newId, uniqueKey, uniqueName, type FlowNode } from './graph';

const FORMAT = 'veylora.clipboard';
let memory: string | null = null;

interface ClipboardPayload {
  format: typeof FORMAT;
  nodes: FlowNode[];
  edges: Edge[];
}

/** Serialises the selected nodes and the edges between them. */
export function serializeSelection(nodes: FlowNode[], edges: Edge[]): string | null {
  const selected = nodes.filter((n) => n.selected);
  if (!selected.length) return null;
  const ids = new Set(selected.map((n) => n.id));
  const payload: ClipboardPayload = {
    format: FORMAT,
    nodes: selected.map((n) => ({ ...n, selected: false, dragging: false })),
    edges: edges.filter((e) => ids.has(e.source) && ids.has(e.target)),
  };
  return JSON.stringify(payload);
}

export async function writeClipboard(text: string): Promise<void> {
  memory = text;
  try {
    await navigator.clipboard?.writeText(text);
  } catch {
    /* system clipboard unavailable — the in-memory copy still works */
  }
}

export async function readClipboard(): Promise<ClipboardPayload | null> {
  let text: string | null;
  try {
    text = (await navigator.clipboard?.readText()) ?? null;
  } catch {
    text = null;
  }
  for (const candidate of [text, memory]) {
    if (!candidate) continue;
    try {
      const parsed = JSON.parse(candidate) as ClipboardPayload;
      if (parsed?.format === FORMAT && Array.isArray(parsed.nodes)) return parsed;
    } catch {
      /* not ours */
    }
  }
  return null;
}

function renameRefs(value: unknown, renames: Map<string, string>): unknown {
  if (typeof value === 'string') {
    let s = value;
    for (const [from, to] of renames)
      s = s.replaceAll(`{{${from}.`, `{{${to}.`).replaceAll(`{{ ${from}.`, `{{ ${to}.`);
    return s;
  }
  if (Array.isArray(value)) return value.map((v) => renameRefs(v, renames));
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, renameRefs(v, renames)]));
  }
  return value;
}

/**
 * Prepares pasted nodes: new IDs, unique names and variable keys, positions offset
 * (or centred at `at`), and {{oldKey.x}} references inside the pasted set renamed.
 */
export function preparePaste(
  payload: ClipboardPayload,
  existing: FlowNode[],
  at?: { x: number; y: number },
): { nodes: FlowNode[]; edges: Edge[] } {
  const idMap = new Map<string, string>();
  const keyMap = new Map<string, string>();
  const placed: FlowNode[] = [...existing];
  const minX = Math.min(...payload.nodes.map((n) => n.position.x));
  const minY = Math.min(...payload.nodes.map((n) => n.position.y));
  const dx = at ? at.x - minX : 40;
  const dy = at ? at.y - minY : 40;

  const fresh = payload.nodes.map((n) => {
    const prefix = n.data.key.replace(/\d+$/, '') || 'node';
    const id = newId(prefix);
    const key = uniqueKey(prefix, placed);
    idMap.set(n.id, id);
    if (key !== n.data.key) keyMap.set(n.data.key, key);
    const node: FlowNode = {
      ...n,
      id,
      selected: true,
      position: { x: n.position.x + dx, y: n.position.y + dy },
      data: { ...structuredClone(n.data), key, name: uniqueName(n.data.name, placed) },
    };
    placed.push(node);
    return node;
  });

  for (const n of fresh) n.data.params = renameRefs(n.data.params, keyMap) as FlowNode['data']['params'];

  const edges = payload.edges
    .filter((e) => idMap.has(e.source) && idMap.has(e.target))
    .map((e) => ({
      ...e,
      id: newId('e'),
      source: idMap.get(e.source)!,
      target: idMap.get(e.target)!,
      selected: false,
    }));
  return { nodes: fresh, edges };
}
