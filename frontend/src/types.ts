// Plain TypeScript mirrors of the Go structures exchanged over Wails bindings.
// Keep in sync with internal/{nodes,workflow,storage,events,validation}.

export type NodeStatus =
  'IDLE' | 'WAITING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED' | 'CANCELLED' | 'TIMEOUT';
export type LogLevel = 'DEBUG' | 'INFO' | 'WARNING' | 'ERROR';

export interface Port {
  id: string;
  label: string;
  kind: string;
  role?: 'error';
}

export interface ParamOption {
  value: string;
  label: string;
}

export type ParamKind = 'string' | 'text' | 'number' | 'select' | 'keyvalue' | 'boolean' | 'workflow';

export interface Template {
  id: string;
  name: string;
  description: string;
  nodes: number;
  triggers: string[];
}

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
  trigger?: 'manual' | 'cron' | 'interval' | 'schedule' | 'file' | 'startup';
}

export type OnError = 'stop' | 'continue' | 'error_output';

export interface NodeSettings {
  retry: { maxAttempts: number; delayMs: number; backoff: boolean; maxDelayMs: number };
  timeoutSec: number;
  onError: OnError;
}

export interface WorkflowVariable {
  key: string;
  value: string;
}

export interface WorkflowSettings {
  timeoutSec: number;
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
  settings?: NodeSettings;
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
  variables: WorkflowVariable[];
  settings: WorkflowSettings;
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
  active: boolean;
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
  trigger: string;
  triggerNodeId: string;
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
  trigger: string;
  triggerNodeId?: string;
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
  attempts?: number;
  handled?: boolean;
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
  maxConcurrentRuns: number;
  allowedDirectories: string[];
  allowedEnvVars: string[];
}

export interface TriggerInfo {
  workflowId: string;
  nodeId: string;
  nodeName: string;
  kind: string;
  nextRun: number;
  detail: string;
}

export interface ActiveRun {
  runId: string;
  workflowId: string;
  startedAt: number;
}

export interface SaveResult {
  updatedAt: number;
  warning?: string;
}

export interface SettingsResult {
  settings: Settings;
  warnings: string[];
}

export interface ActivationResult {
  active: boolean;
  triggers: TriggerInfo[];
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
  settings?: NodeSettings;
}

export interface VersionMeta {
  id: number;
  workflowId: string;
  number: number;
  description: string;
  source: 'manual' | 'auto' | 'restore' | 'import';
  nodes: number;
  edges: number;
  createdAt: number;
}

export interface DiffResult {
  addedNodes: { id: string; name: string; type: string }[];
  removedNodes: { id: string; name: string; type: string }[];
  changedNodes: { id: string; name: string; fields: string[] }[];
  addedEdges: number;
  removedEdges: number;
  variablesChanged: boolean;
  settingsChanged: boolean;
  identical: boolean;
}
