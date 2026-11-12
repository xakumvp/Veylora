import type { Edge } from '@xyflow/react';
import type { FlowNode } from './graph';

const COL_WIDTH = 300;
const ROW_HEIGHT = 150;

/**
 * Layered left-to-right layout: each node goes one column after its deepest
 * predecessor (longest path); rows keep the previous vertical order.
 * Disconnected nodes are placed below the graph.
 */
export function autoLayout(nodes: FlowNode[], edges: Edge[]): Map<string, { x: number; y: number }> {
  const ids = new Set(nodes.map((n) => n.id));
  const incoming = new Map<string, string[]>();
  const outgoing = new Map<string, string[]>();
  for (const e of edges) {
    if (!ids.has(e.source) || !ids.has(e.target)) continue;
    incoming.set(e.target, [...(incoming.get(e.target) ?? []), e.source]);
    outgoing.set(e.source, [...(outgoing.get(e.source) ?? []), e.target]);
  }

  // Longest-path layering via Kahn's algorithm (graphs are acyclic; leftovers get layer 0).
  const layer = new Map<string, number>();
  const indeg = new Map(nodes.map((n) => [n.id, incoming.get(n.id)?.length ?? 0]));
  const queue = nodes.filter((n) => (indeg.get(n.id) ?? 0) === 0).map((n) => n.id);
  queue.forEach((id) => layer.set(id, 0));
  while (queue.length) {
    const id = queue.shift()!;
    for (const t of outgoing.get(id) ?? []) {
      layer.set(t, Math.max(layer.get(t) ?? 0, (layer.get(id) ?? 0) + 1));
      indeg.set(t, (indeg.get(t) ?? 1) - 1);
      if (indeg.get(t) === 0) queue.push(t);
    }
  }

  const connected = nodes.filter((n) => incoming.has(n.id) || outgoing.has(n.id));
  const isolated = nodes.filter((n) => !incoming.has(n.id) && !outgoing.has(n.id));

  const columns = new Map<number, FlowNode[]>();
  for (const n of connected) {
    const l = layer.get(n.id) ?? 0;
    columns.set(l, [...(columns.get(l) ?? []), n]);
  }

  const originX = Math.min(...nodes.map((n) => n.position.x), 0);
  const originY = Math.min(...nodes.map((n) => n.position.y), 0);
  const out = new Map<string, { x: number; y: number }>();
  // Columns left to right; within a column order by the average row of
  // predecessors (already placed), then by current y — reduces edge crossings.
  for (const l of [...columns.keys()].sort((a, b) => a - b)) {
    const col = columns.get(l)!;
    const rank = (n: FlowNode) => {
      const ys = (incoming.get(n.id) ?? [])
        .map((p) => out.get(p)?.y)
        .filter((y): y is number => y !== undefined);
      return ys.length ? ys.reduce((sum, y) => sum + y, 0) / ys.length : n.position.y;
    };
    col.sort((a, b) => rank(a) - rank(b) || a.position.y - b.position.y);
    const offset = -((col.length - 1) * ROW_HEIGHT) / 2;
    col.forEach((n, i) =>
      out.set(n.id, { x: originX + l * COL_WIDTH, y: originY + 200 + offset + i * ROW_HEIGHT }),
    );
  }

  const bottom = Math.max(...[...out.values()].map((p) => p.y), originY) + ROW_HEIGHT * 1.5;
  isolated.forEach((n, i) =>
    out.set(n.id, { x: originX + (i % 4) * COL_WIDTH, y: bottom + Math.floor(i / 4) * ROW_HEIGHT }),
  );
  return out;
}
