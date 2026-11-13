// Typed facade over the generated Wails bindings (src/wailsjs is generated — do not edit it).
import * as Go from '../wailsjs/go/app/App';
import type {
  ActivationResult,
  ActiveRun,
  DiffResult,
  AppInfo,
  Execution,
  ExportResult,
  ImportResult,
  Issue,
  LogEntry,
  NodeDefinition,
  Project,
  SaveResult,
  Settings,
  SettingsResult,
  Template,
  TriggerInfo,
  VersionMeta,
  Workflow,
  WorkflowDocument,
  WorkflowMeta,
} from '../types';

// The generated signatures use classes; our plain objects are structurally identical.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const asGo = <T>(v: T) => v as any;

export const api = {
  appInfo: () => Go.GetAppInfo() as Promise<AppInfo>,
  nodeDefinitions: () => Go.GetNodeDefinitions() as unknown as Promise<NodeDefinition[]>,

  listProjects: () => Go.ListProjects() as Promise<Project[]>,
  createProject: (name: string) => Go.CreateProject(name) as Promise<Project>,
  renameProject: (id: string, name: string) => Go.RenameProject(id, name),
  deleteProject: (id: string) => Go.DeleteProject(id),

  listWorkflows: (projectId: string) => Go.ListWorkflows(projectId) as Promise<WorkflowMeta[]>,
  createWorkflow: (projectId: string, name: string) =>
    Go.CreateWorkflow(projectId, name) as unknown as Promise<Workflow>,
  getWorkflow: (id: string) => Go.GetWorkflow(id) as unknown as Promise<Workflow>,
  saveWorkflow: (id: string, doc: WorkflowDocument) => Go.SaveWorkflow(id, asGo(doc)) as Promise<SaveResult>,
  renameWorkflow: (id: string, name: string, description: string) => Go.RenameWorkflow(id, name, description),
  deleteWorkflow: (id: string) => Go.DeleteWorkflow(id),
  validateWorkflow: (doc: WorkflowDocument) => Go.ValidateWorkflow(asGo(doc)) as Promise<Issue[]>,

  runWorkflow: (workflowId: string, doc: WorkflowDocument, triggerNodeId = '') =>
    Go.RunWorkflow(workflowId, asGo(doc), triggerNodeId),
  stopWorkflow: (workflowId: string) => Go.StopWorkflow(workflowId),
  stopRun: (runId: string) => Go.StopRun(runId),
  activeRuns: () => Go.GetActiveRuns() as Promise<ActiveRun[]>,
  setActive: (workflowId: string, active: boolean) =>
    Go.SetWorkflowActive(workflowId, active) as unknown as Promise<ActivationResult>,
  triggers: (workflowId: string) => Go.GetTriggers(workflowId) as Promise<TriggerInfo[]>,
  pickDirectory: (title: string) => Go.PickDirectory(title),
  startupNotices: () => Go.GetStartupNotices(),
  listExecutions: (workflowId: string) => Go.ListExecutions(workflowId) as Promise<Execution[]>,
  executionLogs: (executionId: string) => Go.GetExecutionLogs(executionId) as Promise<LogEntry[]>,
  clearExecutions: (workflowId: string) => Go.ClearExecutions(workflowId),

  exportWorkflow: (id: string) => Go.ExportWorkflow(id) as Promise<ExportResult | null>,
  importWorkflow: (projectId: string) =>
    Go.ImportWorkflow(projectId) as unknown as Promise<ImportResult | null>,

  createVersion: (workflowId: string, description: string) =>
    Go.CreateVersion(workflowId, description) as Promise<VersionMeta>,
  listVersions: (workflowId: string) => Go.ListVersions(workflowId) as Promise<VersionMeta[]>,
  compareVersion: (workflowId: string, versionId: number) =>
    Go.CompareVersion(workflowId, versionId) as unknown as Promise<DiffResult>,
  restoreVersion: (workflowId: string, versionId: number) =>
    Go.RestoreVersion(workflowId, versionId) as unknown as Promise<Workflow>,
  exportVersion: (workflowId: string, versionId: number) =>
    Go.ExportVersion(workflowId, versionId) as Promise<ExportResult | null>,

  templates: () => Go.GetTemplates() as Promise<Template[]>,
  createFromTemplate: (projectId: string, templateId: string, name: string) =>
    Go.CreateWorkflowFromTemplate(projectId, templateId, name) as Promise<WorkflowMeta>,

  getSettings: () => Go.GetSettings() as Promise<Settings>,
  saveSettings: (s: Settings) => Go.SaveSettings(asGo(s)) as unknown as Promise<SettingsResult>,
};

/** Extracts a readable message from a rejected Wails call. */
export function errorMessage(err: unknown): string {
  if (typeof err === 'string') return err;
  if (err instanceof Error) return err.message;
  try {
    return JSON.stringify(err);
  } catch {
    return 'Неизвестная ошибка';
  }
}
