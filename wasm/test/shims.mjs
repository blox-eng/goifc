// Test-only shims: a browser-style Worker over node:worker_threads, and a
// fetch that serves file: URLs. Install them before importing goifc.js.
import { Worker as NodeWorker } from "node:worker_threads";

// The code each worker thread runs first: browser globals over parentPort,
// then the module that Worker was given. A request whose text holds
// CRASH_MARKER kills the thread, so a test can see a worker die mid-request.
export const CRASH_MARKER = "__goifc_test_crash__";

// A request whose text holds UNCLONABLE_MARKER makes postMessage throw, as
// a browser does for a value it cannot clone.
export const UNCLONABLE_MARKER = "__goifc_test_unclonable__";

// Set stall.next to make the next worker never load, as a hung fetch does.
export const stall = { next: false };

// How many workers were started and terminated, so a test can see a leak.
export const workerCount = { started: 0, terminated: 0 };

const bootstrap = `
const { parentPort, workerData } = require("node:worker_threads");
const fs = require("node:fs");
const { fileURLToPath } = require("node:url");
globalThis.self = globalThis;
globalThis.postMessage = (m, transfer) => parentPort.postMessage(m, transfer);
globalThis.addEventListener = (type, fn) => {
  if (type !== "message") return;
  parentPort.on("message", (data) => {
    if (typeof data?.request === "string" && data.request.includes(${JSON.stringify(CRASH_MARKER)})) process.exit(70);
    fn({ data });
  });
};
const realFetch = globalThis.fetch;
globalThis.fetch = async (url) => {
  const u = String(url);
  if (!u.startsWith("file:")) return realFetch(url);
  return new Response(fs.readFileSync(fileURLToPath(u)), { headers: { "content-type": "application/wasm" } });
};
if (!workerData.stall) import(workerData.url);
`;

class Worker extends EventTarget {
  #thread;
  #terminated = false;
  #stopped = false;
  constructor(url) {
    super();
    workerCount.started++;
    const stalled = stall.next;
    stall.next = false;
    this.#thread = new NodeWorker(bootstrap, { eval: true, workerData: { url: String(url), stall: stalled } });
    this.#thread.on("message", (data) => this.dispatchEvent(new MessageEvent("message", { data })));
    this.#thread.on("error", (err) => this.#fail(err.message));
    this.#thread.on("exit", (code) => this.#fail(`the worker thread exited with code ${code}`));
  }
  #fail(message) {
    if (this.#terminated) return;
    this.#terminated = true;
    const e = new Event("error");
    e.message = message;
    this.dispatchEvent(e);
  }
  postMessage(message, transfer) {
    if (typeof message?.request === "string" && message.request.includes(UNCLONABLE_MARKER)) {
      throw new DOMException("the message could not be cloned", "DataCloneError");
    }
    this.#thread.postMessage(message, transfer);
  }
  terminate() {
    if (!this.#stopped) workerCount.terminated++;
    this.#stopped = true;
    this.#terminated = true;
    this.#thread.terminate();
  }
}

globalThis.Worker = Worker;
