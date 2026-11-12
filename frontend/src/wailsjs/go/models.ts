export namespace app {
	
	export class ActivationResult {
	    active: boolean;
	    triggers: scheduler.TriggerInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ActivationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.triggers = this.convertValues(source["triggers"], scheduler.TriggerInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppInfo {
	    name: string;
	    version: string;
	    dataDir: string;
	    logFile: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.logFile = source["logFile"];
	    }
	}
	export class ExportResult {
	    path: string;
	    removedSecrets: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.removedSecrets = source["removedSecrets"];
	    }
	}
	export class ImportResult {
	    workflow: storage.WorkflowMeta;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflow = this.convertValues(source["workflow"], storage.WorkflowMeta);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SaveResult {
	    updatedAt: number;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.updatedAt = source["updatedAt"];
	        this.warning = source["warning"];
	    }
	}
	export class SettingsResult {
	    settings: storage.Settings;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new SettingsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], storage.Settings);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace engine {
	
	export class ActiveRun {
	    runId: string;
	    workflowId: string;
	    startedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ActiveRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.workflowId = source["workflowId"];
	        this.startedAt = source["startedAt"];
	    }
	}

}

export namespace events {
	
	export class Log {
	    id?: number;
	    runId: string;
	    nodeId?: string;
	    nodeName?: string;
	    level: string;
	    message: string;
	    data?: any;
	    timestamp: number;
	
	    static createFrom(source: any = {}) {
	        return new Log(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.runId = source["runId"];
	        this.nodeId = source["nodeId"];
	        this.nodeName = source["nodeName"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.data = source["data"];
	        this.timestamp = source["timestamp"];
	    }
	}

}

export namespace nodes {
	
	export class Field {
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new Field(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class ParamCondition {
	    param: string;
	    in: string[];
	
	    static createFrom(source: any = {}) {
	        return new ParamCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.param = source["param"];
	        this.in = source["in"];
	    }
	}
	export class Option {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Option(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class ParamSpec {
	    key: string;
	    label: string;
	    kind: string;
	    description?: string;
	    placeholder?: string;
	    required: boolean;
	    default?: any;
	    options?: Option[];
	    min?: number;
	    max?: number;
	    templatable: boolean;
	    optionalWhen?: ParamCondition;
	
	    static createFrom(source: any = {}) {
	        return new ParamSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.placeholder = source["placeholder"];
	        this.required = source["required"];
	        this.default = source["default"];
	        this.options = this.convertValues(source["options"], Option);
	        this.min = source["min"];
	        this.max = source["max"];
	        this.templatable = source["templatable"];
	        this.optionalWhen = this.convertValues(source["optionalWhen"], ParamCondition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Port {
	    id: string;
	    label: string;
	    kind: string;
	    role?: string;
	
	    static createFrom(source: any = {}) {
	        return new Port(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.role = source["role"];
	    }
	}
	export class Definition {
	    type: string;
	    name: string;
	    category: string;
	    description: string;
	    icon: string;
	    keyPrefix: string;
	    inputs: Port[];
	    outputs: Port[];
	    params: ParamSpec[];
	    outputDoc: Field[];
	    trigger?: string;
	
	    static createFrom(source: any = {}) {
	        return new Definition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.keyPrefix = source["keyPrefix"];
	        this.inputs = this.convertValues(source["inputs"], Port);
	        this.outputs = this.convertValues(source["outputs"], Port);
	        this.params = this.convertValues(source["params"], ParamSpec);
	        this.outputDoc = this.convertValues(source["outputDoc"], Field);
	        this.trigger = source["trigger"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	

}

export namespace scheduler {
	
	export class TriggerInfo {
	    workflowId: string;
	    nodeId: string;
	    nodeName: string;
	    kind: string;
	    nextRun: number;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new TriggerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflowId = source["workflowId"];
	        this.nodeId = source["nodeId"];
	        this.nodeName = source["nodeName"];
	        this.kind = source["kind"];
	        this.nextRun = source["nextRun"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace storage {
	
	export class Execution {
	    id: string;
	    workflowId: string;
	    status: string;
	    error: string;
	    trigger: string;
	    triggerNodeId: string;
	    startedAt: number;
	    finishedAt: number;
	    durationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Execution(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflowId = source["workflowId"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.trigger = source["trigger"];
	        this.triggerNodeId = source["triggerNodeId"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.durationMs = source["durationMs"];
	    }
	}
	export class Project {
	    id: string;
	    name: string;
	    description: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Settings {
	    allowLocalNetwork: boolean;
	    maxResponseMb: number;
	    keepExecutions: number;
	    maxConcurrentRuns: number;
	    allowedDirectories: string[];
	    allowedEnvVars: string[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowLocalNetwork = source["allowLocalNetwork"];
	        this.maxResponseMb = source["maxResponseMb"];
	        this.keepExecutions = source["keepExecutions"];
	        this.maxConcurrentRuns = source["maxConcurrentRuns"];
	        this.allowedDirectories = source["allowedDirectories"];
	        this.allowedEnvVars = source["allowedEnvVars"];
	    }
	}
	export class VersionMeta {
	    id: number;
	    workflowId: string;
	    number: number;
	    description: string;
	    source: string;
	    nodes: number;
	    edges: number;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new VersionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflowId = source["workflowId"];
	        this.number = source["number"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.nodes = source["nodes"];
	        this.edges = source["edges"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Workflow {
	    id: string;
	    projectId: string;
	    name: string;
	    description: string;
	    active: boolean;
	    createdAt: number;
	    updatedAt: number;
	    document: workflow.Document;
	
	    static createFrom(source: any = {}) {
	        return new Workflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.active = source["active"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.document = this.convertValues(source["document"], workflow.Document);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkflowMeta {
	    id: string;
	    projectId: string;
	    name: string;
	    description: string;
	    active: boolean;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.active = source["active"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace validation {
	
	export class Issue {
	    severity: string;
	    message: string;
	    nodeId?: string;
	    edgeId?: string;
	    field?: string;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.message = source["message"];
	        this.nodeId = source["nodeId"];
	        this.edgeId = source["edgeId"];
	        this.field = source["field"];
	    }
	}

}

export namespace workflow {
	
	export class NodeChange {
	    id: string;
	    name: string;
	    fields: string[];
	
	    static createFrom(source: any = {}) {
	        return new NodeChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.fields = source["fields"];
	    }
	}
	export class NodeRef {
	    id: string;
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class DiffResult {
	    addedNodes: NodeRef[];
	    removedNodes: NodeRef[];
	    changedNodes: NodeChange[];
	    addedEdges: number;
	    removedEdges: number;
	    variablesChanged: boolean;
	    settingsChanged: boolean;
	    identical: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.addedNodes = this.convertValues(source["addedNodes"], NodeRef);
	        this.removedNodes = this.convertValues(source["removedNodes"], NodeRef);
	        this.changedNodes = this.convertValues(source["changedNodes"], NodeChange);
	        this.addedEdges = source["addedEdges"];
	        this.removedEdges = source["removedEdges"];
	        this.variablesChanged = source["variablesChanged"];
	        this.settingsChanged = source["settingsChanged"];
	        this.identical = source["identical"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Viewport {
	    x: number;
	    y: number;
	    zoom: number;
	
	    static createFrom(source: any = {}) {
	        return new Viewport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.zoom = source["zoom"];
	    }
	}
	export class Settings {
	    timeoutSec: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSec = source["timeoutSec"];
	    }
	}
	export class Variable {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Variable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class Edge {
	    id: string;
	    source: string;
	    sourceHandle: string;
	    target: string;
	    targetHandle: string;
	
	    static createFrom(source: any = {}) {
	        return new Edge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.source = source["source"];
	        this.sourceHandle = source["sourceHandle"];
	        this.target = source["target"];
	        this.targetHandle = source["targetHandle"];
	    }
	}
	export class Position {
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new Position(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class Retry {
	    maxAttempts: number;
	    delayMs: number;
	    backoff: boolean;
	    maxDelayMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Retry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxAttempts = source["maxAttempts"];
	        this.delayMs = source["delayMs"];
	        this.backoff = source["backoff"];
	        this.maxDelayMs = source["maxDelayMs"];
	    }
	}
	export class NodeSettings {
	    retry: Retry;
	    timeoutSec: number;
	    onError: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.retry = this.convertValues(source["retry"], Retry);
	        this.timeoutSec = source["timeoutSec"];
	        this.onError = source["onError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Node {
	    id: string;
	    type: string;
	    name: string;
	    key: string;
	    params: Record<string, any>;
	    settings?: NodeSettings;
	    position: Position;
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.key = source["key"];
	        this.params = source["params"];
	        this.settings = this.convertValues(source["settings"], NodeSettings);
	        this.position = this.convertValues(source["position"], Position);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Document {
	    schemaVersion: number;
	    nodes: Node[];
	    edges: Edge[];
	    variables: Variable[];
	    settings: Settings;
	    viewport?: Viewport;
	
	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.nodes = this.convertValues(source["nodes"], Node);
	        this.edges = this.convertValues(source["edges"], Edge);
	        this.variables = this.convertValues(source["variables"], Variable);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.viewport = this.convertValues(source["viewport"], Viewport);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	

}

