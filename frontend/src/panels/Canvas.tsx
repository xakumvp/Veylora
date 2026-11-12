import { useCallback, useEffect, useMemo, type DragEvent } from 'react';
import {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Connection,
  type Edge,
  type IsValidConnection,
} from '@xyflow/react';
import { MousePointerClick, Workflow as WorkflowIcon } from 'lucide-react';
import { Kbd } from '../components/ui';
import { categoryColor } from '../components/icons';
import { canvasBridge, DND_MIME } from '../lib/canvasBridge';
import { NODE_TYPE, wouldCreateCycle, type FlowNode } from '../lib/graph';
import { useEditor } from '../stores/editor';
import { useRun } from '../stores/run';
import { useWorkspace } from '../stores/workspace';
import { toast, useUI } from '../stores/ui';
import { VeyloraNode } from '../nodes/VeyloraNode';

const nodeTypes = { [NODE_TYPE]: VeyloraNode };

function FlowCanvas() {
  const nodes = useEditor((s) => s.nodes);
  const edges = useEditor((s) => s.edges);
  const viewport = useEditor((s) => s.viewport);
  const onNodesChange = useEditor((s) => s.onNodesChange);
  const onEdgesChange = useEditor((s) => s.onEdgesChange);
  const connect = useEditor((s) => s.connect);
  const beginDrag = useEditor((s) => s.beginDrag);
  const setViewport = useEditor((s) => s.setViewport);
  const addNode = useEditor((s) => s.addNode);
  const defByType = useWorkspace((s) => s.defByType);
  const runNodes = useRun((s) => s.nodes);
  const flow = useReactFlow<FlowNode, Edge>();

  useEffect(() => {
    canvasBridge.set({
      centerPosition: () => {
        const el = document.querySelector('.react-flow');
        const rect = el?.getBoundingClientRect();
        const jitter = () => Math.round((Math.random() - 0.5) * 60);
        if (!rect) return { x: 200, y: 200 };
        const p = flow.screenToFlowPosition({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 });
        return { x: p.x - 120 + jitter(), y: p.y - 40 + jitter() };
      },
      fitView: () => void flow.fitView({ padding: 0.12, maxZoom: 1, duration: 300 }),
      focusNode: (id) => {
        const n = flow.getNode(id);
        if (!n) return;
        useEditor.getState().selectOnly(id);
        void flow.setCenter(n.position.x + 120, n.position.y + 50, {
          zoom: Math.max(flow.getZoom(), 1),
          duration: 300,
        });
      },
    });
    return () => canvasBridge.set(null);
  }, [flow]);

  // Highlight the path the engine actually took.
  const styledEdges = useMemo(() => {
    if (!Object.keys(runNodes).length) return edges;
    return edges.map((e) => {
      const src = runNodes[e.source];
      const tgt = runNodes[e.target];
      const isErrorEdge = e.sourceHandle === 'error';
      const taken = isErrorEdge
        ? src?.status === 'FAILED' && src.branch === 'error'
        : src?.status === 'SUCCESS' && (!src.branch || src.branch === e.sourceHandle);
      const skipped =
        tgt?.status === 'SKIPPED' ||
        (src?.status === 'SUCCESS' && (isErrorEdge || (!!src.branch && src.branch !== e.sourceHandle)));
      return {
        ...e,
        className: taken && !skipped ? 'taken' : skipped ? 'skipped' : undefined,
        animated: taken && tgt?.status === 'RUNNING',
      };
    });
  }, [edges, runNodes]);

  const isValidConnection: IsValidConnection<Edge> = useCallback(
    (c) => {
      const { nodes: ns, edges: es } = useEditor.getState();
      if (!c.source || !c.target || c.source === c.target) return false;
      const src = ns.find((n) => n.id === c.source);
      const tgt = ns.find((n) => n.id === c.target);
      if (!src || !tgt) return false;
      const out = defByType[src.data.nodeType]?.outputs.find((p) => p.id === c.sourceHandle);
      const inp = defByType[tgt.data.nodeType]?.inputs.find((p) => p.id === c.targetHandle);
      if (!out || !inp || out.kind !== inp.kind) return false;
      if (es.some((e) => e.source === c.source && e.sourceHandle === c.sourceHandle && e.target === c.target))
        return false;
      return !wouldCreateCycle(es, c.source, c.target);
    },
    [defByType],
  );

  const onConnect = useCallback(
    (c: Connection) => {
      if (c.source && c.target && wouldCreateCycle(useEditor.getState().edges, c.source, c.target)) {
        toast('warning', 'Соединение создаёт цикл', 'Циклы не поддерживаются в MVP');
        return;
      }
      connect(c);
    },
    [connect],
  );

  const onDragOver = useCallback((e: DragEvent) => {
    if (e.dataTransfer.types.includes(DND_MIME)) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'copy';
    }
  }, []);

  const onDrop = useCallback(
    (e: DragEvent) => {
      const type = e.dataTransfer.getData(DND_MIME);
      const def = defByType[type];
      if (!def) return;
      e.preventDefault();
      const p = flow.screenToFlowPosition({ x: e.clientX, y: e.clientY });
      addNode(def, { x: p.x - 120, y: p.y - 30 });
    },
    [addNode, defByType, flow],
  );

  return (
    <ReactFlow<FlowNode, Edge>
      nodes={nodes}
      edges={styledEdges}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      isValidConnection={isValidConnection}
      onNodeDragStart={beginDrag}
      onSelectionDragStart={beginDrag}
      onMoveEnd={(event, vp) => event && setViewport(vp)}
      onDrop={onDrop}
      onNodeContextMenu={(e, node) => {
        e.preventDefault();
        if (!node.selected) useEditor.getState().selectOnly(node.id);
        useUI.getState().openContextMenu({ x: e.clientX, y: e.clientY, kind: 'node', nodeId: node.id });
      }}
      onSelectionContextMenu={(e, sel) => {
        e.preventDefault();
        useUI.getState().openContextMenu({ x: e.clientX, y: e.clientY, kind: 'node', nodeId: sel[0]?.id });
      }}
      onPaneContextMenu={(e) => {
        e.preventDefault();
        useUI.getState().openContextMenu({
          x: e.clientX,
          y: e.clientY,
          kind: 'pane',
          flow: flow.screenToFlowPosition({ x: e.clientX, y: e.clientY }),
        });
      }}
      onPaneClick={() => useUI.getState().openContextMenu(null)}
      onDragOver={onDragOver}
      defaultViewport={viewport}
      fitView={!viewport}
      fitViewOptions={{ maxZoom: 1, padding: 0.08 }}
      minZoom={0.2}
      maxZoom={2}
      colorMode="dark"
      deleteKeyCode={['Delete', 'Backspace']}
      multiSelectionKeyCode={['Control', 'Meta']}
      selectionKeyCode="Shift"
      snapToGrid
      snapGrid={[10, 10]}
      defaultEdgeOptions={{ interactionWidth: 18 }}
      connectionRadius={28}
      proOptions={{ hideAttribution: true }}
    >
      <Background variant={BackgroundVariant.Dots} gap={20} size={1.4} />
      <Controls position="bottom-left" showInteractive={false} />
      <MiniMap
        position="bottom-right"
        pannable
        zoomable
        nodeColor={(n) =>
          categoryColor[defByType[(n as FlowNode).data?.nodeType]?.category ?? ''] ?? '#3a3d48'
        }
        nodeStrokeWidth={0}
        nodeBorderRadius={6}
        style={{ width: 168, height: 112 }}
      />
      {nodes.length === 0 && <EmptyHint />}
    </ReactFlow>
  );
}

function EmptyHint() {
  return (
    <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center">
      <div className="flex max-w-xs flex-col items-center gap-2 text-center text-muted">
        <MousePointerClick size={28} className="text-faint" />
        <p className="text-sm text-fg">Перетащите Node из библиотеки</p>
        <p className="text-xs">или кликните по нему, чтобы добавить в центр холста</p>
      </div>
    </div>
  );
}

export function Canvas() {
  const workflowId = useEditor((s) => s.workflowId);
  const opening = useWorkspace((s) => s.opening);

  if (!workflowId) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex max-w-sm flex-col items-center gap-3 text-center">
          <div className="flex size-12 items-center justify-center rounded-xl bg-accent-soft text-accent">
            <WorkflowIcon size={22} />
          </div>
          <p className="text-sm font-medium">{opening ? 'Открываю Workflow…' : 'Workflow не выбран'}</p>
          <p className="text-xs leading-relaxed text-muted">
            Выберите Workflow в боковой панели или создайте новый. Подсказки: <Kbd>Ctrl</Kbd>+<Kbd>S</Kbd> —
            сохранить, <Kbd>Ctrl</Kbd>+<Kbd>Enter</Kbd> — запустить.
          </p>
        </div>
      </div>
    );
  }

  return (
    <ReactFlowProvider key={workflowId}>
      <FlowCanvas />
    </ReactFlowProvider>
  );
}
