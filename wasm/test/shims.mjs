// Test-only shims: a browser-style Worker over node:worker_threads, and a
// fetch that serves file: URLs. Install them before importing goifc.js.
import { Worker as NodeWorker } from "node:worker_threads";

// The code each worker thread runs first: browser globals over parentPort,
// then the module that Worker was given. A request whose text holds
// CRASH_MARKER kills the thread, so a test can see a worker die mid-request.
export const CRASH_MARKER = "__goifc_test_crash__";

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
import(workerData.url);
`;

class Worker extends EventTarget {
  #thread;
  #terminated = false;
  constructor(url) {
    super();
    this.#thread = new NodeWorker(bootstrap, { eval: true, workerData: { url: String(url) } });
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
    this.#thread.postMessage(message, transfer);
  }
  terminate() {
    this.#terminated = true;
    this.#thread.terminate();
  }
}

globalThis.Worker = Worker;
