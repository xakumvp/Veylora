import { EventsOn } from '../wailsjs/runtime/runtime';
import type { LogEntry, NodeEvent, RunEvent } from '../types';

// Keep in sync with internal/events/events.go.
export const EVENTS = {
  runStarted: 'run:started',
  runFinished: 'run:finished',
  nodeStatus: 'node:status',
  logEntry: 'log:entry',
  schedulerChanged: 'scheduler:changed',
  triggerSkipped: 'trigger:skipped',
} as const;

export interface TriggerSkippedEvent {
  workflowId: string;
  nodeId: string;
  reason: string;
}

export interface EngineHandlers {
  onRunStarted: (ev: RunEvent) => void;
  onRunFinished: (ev: RunEvent) => void;
  onNodeStatus: (ev: NodeEvent) => void;
  onLog: (l: LogEntry) => void;
  onSchedulerChanged: (workflowId: string) => void;
  onTriggerSkipped: (ev: TriggerSkippedEvent) => void;
}

/** Subscribes to all engine events; returns an unsubscribe function. */
export function subscribeEngine(h: EngineHandlers): () => void {
  const offs = [
    EventsOn(EVENTS.runStarted, (ev: RunEvent) => h.onRunStarted(ev)),
    EventsOn(EVENTS.runFinished, (ev: RunEvent) => h.onRunFinished(ev)),
    EventsOn(EVENTS.nodeStatus, (ev: NodeEvent) => h.onNodeStatus(ev)),
    EventsOn(EVENTS.logEntry, (l: LogEntry) => h.onLog(l)),
    EventsOn(EVENTS.schedulerChanged, (id: string) => h.onSchedulerChanged(id)),
    EventsOn(EVENTS.triggerSkipped, (ev: TriggerSkippedEvent) => h.onTriggerSkipped(ev)),
  ];
  return () => offs.forEach((off) => off());
}
