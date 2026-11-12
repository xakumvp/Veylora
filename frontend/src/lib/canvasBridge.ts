// Lets panels outside the React Flow provider (e.g. the Node Library) ask the canvas
// for a good spot to drop a node or to focus a node.
export interface CanvasBridge {
  centerPosition: () => { x: number; y: number };
  focusNode: (id: string) => void;
}

let bridge: CanvasBridge | null = null;

export const canvasBridge = {
  set(b: CanvasBridge | null) {
    bridge = b;
  },
  centerPosition() {
    return bridge?.centerPosition() ?? { x: 200, y: 200 };
  },
  focusNode(id: string) {
    bridge?.focusNode(id);
  },
};

export const DND_MIME = 'application/x-veylora-node';
