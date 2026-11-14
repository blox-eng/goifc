// goifc.js runs goifc-build.wasm or goifc-read.wasm in a Web Worker and
// answers with Promises. The same file is the worker script.
//
//   import { createBuilder, createReader } from "./goifc.js";
//   const builder = await createBuilder({ base: "/static/goifc/" });
//   const { answer, glb } = await builder.build(request);
//
// base is the URL of the folder that holds goifc.js, wasm_exec.js and the
// .wasm files. See docs/guides/wasm.md for the request and answer shapes.

const MAX_READ_BYTES = 50331648; // 48 MiB, as in goifc-read.wasm
const WORKER_PARAM = "goifc-worker";

const files = { build: "goifc-build.wasm", read: "goifc-read.wasm" };

/** Starts a worker with goifc-build.wasm. */
export async function createBuilder({ base } = {}) {
  const w = await start("build", base);
  return {
    /** @returns {Promise<{answer: object, glb: ArrayBuffer | null}>} */
    build: (request) => w.call({ request: JSON.stringify(request ?? {}) }),
    close: w.close,
  };
}

/** Starts a worker with goifc-read.wasm. */
export async function createReader({ base } = {}) {
  const w = await start("read", base);
  return {
    /** @returns {Promise<{answer: object, glb: ArrayBuffer | null}>} */
    read: async (file, { include } = {}) => {
      const size = file?.byteLength ?? file?.size;
      if (typeof size !== "number") {
        throw failure("invalid_input", "read takes a Uint8Array, an ArrayBuffer or a Blob");
      }
      if (size > MAX_READ_BYTES) {
        throw failure("too_large", `the file is ${size.toLocaleString("en-US")} bytes, over the 50,331,648-byte cap`);
      }
      if (include != null && !(Array.isArray(include) && include.every((p) => typeof p === "string"))) {
        throw failure("invalid_input", "include must be an array of part names, or left out for nodes and glb");
      }
      const bytes = await copy(file);
      return w.call({ bytes, include: include ?? null }, [bytes]);
    },
    close: w.close,
  };
}

function failure(code, message) {
  const e = new Error(message);
  e.code = code;
  return e;
}

// copy gives an ArrayBuffer the caller does not own, so transferring it to
// the worker never detaches the caller's data.
async function copy(file) {
  if (typeof Blob !== "undefined" && file instanceof Blob) return file.arrayBuffer();
  const view = file instanceof ArrayBuffer ? new Uint8Array(file) : new Uint8Array(file.buffer, file.byteOffset, file.byteLength);
  return view.slice().buffer;
}

// start returns a queue that sends one request at a time to a worker and
// replaces the worker if it dies. At most one worker runs or starts at a
// time: every caller waits on the same starting promise.
async function start(kind, base) {
  // A base the caller gives resolves against the page; the default is the
  // folder goifc.js was loaded from.
  const baseURL = (base === undefined ? new URL("./", import.meta.url) : new URL(base, globalThis.location?.href ?? import.meta.url)).href;
  const queue = [];
  let worker = null;
  let starting = null;
  let busy = false;
  let closed = false;

  const spawn = () =>
    (starting ??= new Promise((resolve, reject) => {
      const url = new URL(import.meta.url);
      url.searchParams.set(WORKER_PARAM, kind);
      url.searchParams.set("base", baseURL);
      const w = new Worker(url, { type: "module" });
      let ready = false;
      const died = (message) => {
        w.terminate();
        if (!ready) {
          starting = null;
          return reject(failure("worker_failed", message));
        }
        if (w !== worker) return;
        worker = null;
        const job = busy ? queue.shift() : null;
        busy = false;
        job?.reject(failure("worker_failed", message));
        pump();
      };
      w.addEventListener("error", (e) => died(`the ${files[kind]} worker stopped: ${e.message ?? "unknown error"}`));
      w.addEventListener("message", ({ data }) => {
        if (data.type === "ready") {
          ready = true;
          starting = null;
          if (closed) w.terminate();
          else worker = w;
          return resolve();
        }
        if (data.type === "failed" || data.type === "exit") return died(data.message);
        if (w !== worker) return;
        const job = queue.shift();
        busy = false;
        job.resolve({ answer: JSON.parse(data.answer), glb: data.glb });
        pump();
      });
    }));

  const pump = () => {
    if (busy || closed || queue.length === 0) return;
    if (!worker) {
      spawn().then(pump, (e) => {
        for (const job of queue.splice(0)) job.reject(e);
      });
      return;
    }
    const job = queue[0];
    busy = true;
    try {
      worker.postMessage(job.message, job.transfer);
    } catch (e) {
      // The worker never saw this request: refuse it and go on.
      queue.shift();
      busy = false;
      job.reject(e);
      pump();
    }
  };

  await spawn();
  return {
    call: (message, transfer = []) =>
      new Promise((resolve, reject) => {
        if (closed) return reject(failure("worker_failed", "the worker was closed"));
        queue.push({ message, transfer, resolve, reject });
        pump();
      }),
    close: () => {
      closed = true;
      worker?.terminate();
      worker = null;
      for (const job of queue.splice(0)) job.reject(failure("worker_failed", "the worker was closed"));
    },
  };
}

// The worker side: load wasm_exec.js and one .wasm, then answer requests.
const params = new URL(import.meta.url).searchParams;
if (params.has(WORKER_PARAM)) {
  const kind = params.get(WORKER_PARAM);
  const base = params.get("base");
  (async () => {
    await import(new URL("wasm_exec.js", base).href);
    const go = new globalThis.Go();
    const url = new URL(files[kind], base).href;
    let instance;
    try {
      ({ instance } = await WebAssembly.instantiateStreaming(fetch(url), go.importObject));
    } catch {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`fetching ${url}: HTTP ${res.status}`);
      ({ instance } = await WebAssembly.instantiate(await res.arrayBuffer(), go.importObject));
    }
    go.run(instance).then(() => self.postMessage({ type: "exit", message: `${files[kind]} exited` }));
    self.addEventListener("message", ({ data }) => {
      const out =
        kind === "build" ? globalThis.goifcBuild(data.request) : globalThis.goifcRead(new Uint8Array(data.bytes), data.include);
      const glb = out.glb ? out.glb.buffer.slice(out.glb.byteOffset, out.glb.byteOffset + out.glb.byteLength) : null;
      self.postMessage({ answer: out.answer, glb }, glb ? [glb] : []);
    });
    self.postMessage({ type: "ready" });
  })().catch((e) => self.postMessage({ type: "failed", message: `loading ${files[kind]}: ${e.message}` }));
}
