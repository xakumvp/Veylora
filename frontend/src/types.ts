// Plain TypeScript mirrors of the Go structures exchanged over Wails bindings.
// Keep in sync with internal/{nodes,workflow,storage,events,validation}.

export type NodeStatus = 'IDLE' | 'WAITING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED' | 'CANCELLED';
export type LogLevel = 'DEBUG' | 'INFO' | 'WARNING' | 'ERROR';

export interface Port {
  id: string;
  label: string;
  kind: string;
}

export interface ParamOption {
  value: string;
  label: string;
}

export type ParamKind = 'string' | 'text' | 'number' | 'select' | 'keyvalue' | 'boolean';

export interface ParamSpec {
  key: string;
  label: string;
  kind: ParamKind;
  description?: string;
  placeholder?: string;
  required: boolean;
  default?: unknown;
  options?: ParamOption[];
  min?: number;
  max?: number;
  templatable: boolean;
  optionalWhen?: { param: string; in: string[] };
}

export interface NodeDefinition {
  type: string;
  name: string;
  category: string;
  description: string;
  icon: string;
  keyPrefix: string;
  inputs: Port[];
  outputs: Port[];
  params: ParamSpec[];
  outputDoc: { name: string; description: string }[];
}

export interface KeyValueRow {
  key: string;
  value: string;
  enabled: boolean;
}

export type Params = Record<string, unknown>;

export interface WorkflowNode {
  id: string;
  type: string;
  name: string;
  key: string;
  params: Params;
  position: { x: number; y: number };
}

export interface WorkflowEdge {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
}

export interface Viewport {
  x: number;
  y: number;
  zoom: number;
}

export interface WorkflowDocument {
  schemaVersion: number;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  viewport?: Viewport;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  createdAt: number;
  updatedAt: number;
}

export interface WorkflowMeta {
  id: string;
  projectId: string;
  name: string;
  description: string;
  createdAt: number;
  updatedAt: number;
}

export interface Workflow extends WorkflowMeta {
  document: WorkflowDocument;
}

export interface Execution {
  id: string;
  workflowId: string;
  status: NodeStatus;
  error: string;
  startedAt: number;
  finishedAt: number;
  durationMs: number;
}

export interface LogEntry {
  id?: number;
  runId: string;
  nodeId?: string;
  nodeName?: string;
  level: LogLevel;
  message: string;
  data?: unknown;
  timestamp: number;
}

export interface RunEvent {
  runId: string;
  workflowId: string;
  status: NodeStatus;
  startedAt: number;
  finishedAt?: number;
  durationMs: number;
  error?: string;
}

export interface NodeEvent {
  runId: string;
  nodeId: string;
  status: NodeStatus;
  branch?: string;
  error?: string;
  output?: unknown;
  startedAt?: number;
  finishedAt?: number;
  durationMs: number;
}

export interface Issue {
  severity: 'error' | 'warning';
  message: string;
  nodeId?: string;
  edgeId?: string;
  field?: string;
}

export interface Settings {
  allowLocalNetwork: boolean;
  maxResponseMb: number;
  keepExecutions: number;
}

export interface AppInfo {
  name: string;
  version: string;
  dataDir: string;
  logFile: string;
}

export interface ExportResult {
  path: string;
  removedSecrets: string[];
}

export interface ImportResult {
  workflow: WorkflowMeta;
  warnings: string[];
}

/** Data stored on each React Flow node. */
export interface VNodeData extends Record<string, unknown> {
  nodeType: string;
  name: string;
  key: string;
  params: Params;
}
