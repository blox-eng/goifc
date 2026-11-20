// Runs goifc.js and the built .wasm files from dist/ (make wasm) under Node.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { CRASH_MARKER, UNCLONABLE_MARKER, stall, workerCount } from "./shims.mjs";
import { createBuilder, createReader } from "../goifc.js";

const root = new URL("../../", import.meta.url);
const dist = new URL("dist/", root);
const goldens = new URL("internal/wasmapi/testdata/", root);

const read = (url) => readFileSync(url);
const json = (url) => JSON.parse(readFileSync(url, "utf8"));

// The goldens carry a stand-in version; the built file carries its own.
const withVersion = (answer, version) => ({ ...answer, goifcVersion: version });

test("build answers equal the Go goldens", async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    const dir = new URL("build/", goldens);
    const names = readdirSync(dir).filter((f) => f.endsWith(".request.json")).map((f) => f.slice(0, -".request.json".length));
    assert.ok(names.length > 0);
    for (const name of names) {
      const { answer, glb } = await builder.build(json(new URL(`${name}.request.json`, dir)));
      assert.deepEqual(answer, withVersion(json(new URL(`${name}.answer.json`, dir)), answer.goifcVersion), name);
      if (answer.ok) {
        assert.ok(glb instanceof ArrayBuffer, name);
        assert.deepEqual(Buffer.from(glb), read(new URL(`${name}.glb`, dir)), name);
      } else {
        assert.equal(glb, null, name);
      }
    }
  } finally {
    builder.close();
  }
});

test("build a pitched roof and refuse a stepped one", async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    const rect = [[0, 0], [10, 0], [10, 8], [0, 8]];
    const { answer, glb } = await builder.build({ roofs: [{ id: "r1", outline: rect, pitch: 30, eaves: 3, thickness: 0.2 }] });
    assert.equal(answer.ok, true);
    assert.ok(Math.abs(answer.roofs[0].quantities.ridge - 2) < 1e-9);
    assert.equal(answer.roofs[0].planes.length, 4);
    assert.ok(glb instanceof ArrayBuffer);
    const ell = [[0, 0], [18, 0], [18, 6], [12, 6], [12, 10], [0, 10]];
    const refused = await builder.build({ roofs: [{ id: "r2", outline: ell, pitch: 30, gables: [2], eaves: 3, thickness: 0.2 }] });
    assert.equal(refused.answer.ok, false);
    assert.equal(refused.answer.errors[0].code, "roof_step");
    assert.equal(refused.answer.errors[0].item, "r2");
    assert.equal(refused.answer.errors[0].edge, 2);
    assert.equal(refused.answer.errors[0].other, undefined);
  } finally {
    builder.close();
  }
});

test("read answers equal the Go goldens", async () => {
  const reader = await createReader({ base: dist.href });
  try {
    const fixtures = {
      two_storey_spanning: "testdata/two_storey_spanning.ifc",
      type_inheritance: "model/testdata/synthetic/type_inheritance.ifc",
      netarea_rect_window: "geometry/testdata/synthetic/netarea_rect_window.ifc",
      wall_layerset_attrs: "model/testdata/synthetic/wall_layerset_attrs.ifc",
    };
    for (const [name, path] of Object.entries(fixtures)) {
      const bytes = read(new URL(path, root));
      for (const [suffix, include] of [["default", undefined], ["all", ["nodes", "glb", "psets", "storeyPlans", "typeLayers"]]]) {
        const { answer, glb } = await reader.read(new Uint8Array(bytes), include && { include });
        const dir = new URL("read/", goldens);
        assert.deepEqual(answer, withVersion(json(new URL(`${name}.${suffix}.answer.json`, dir)), answer.goifcVersion), `${name} ${suffix}`);
        assert.deepEqual(Buffer.from(glb), read(new URL(`${name}.${suffix}.glb`, dir)), `${name} ${suffix}`);
      }
    }
  } finally {
    reader.close();
  }
});

test("requests resolve in the order they were sent", async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    const slab = (id) => ({ slabs: [{ id, outline: [[0, 0], [1, 0], [1, 1]], z: 0, thickness: 0.2 }] });
    const done = [];
    await Promise.all(["a", "b", "c"].map((id) => builder.build(slab(id)).then(({ answer }) => done.push(answer.slabs[0].id))));
    assert.deepEqual(done, ["a", "b", "c"]);
  } finally {
    builder.close();
  }
});

test("read refuses a file over 48 MiB before it reaches the worker", async () => {
  const reader = await createReader({ base: dist.href });
  try {
    await assert.rejects(reader.read(new Uint8Array(50331649)), (e) => e.code === "too_large" && /50,331,648/.test(e.message));
  } finally {
    reader.close();
  }
});

test("a dead worker rejects with worker_failed, and the next request works", async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    const crash = builder.build({ slabs: [{ id: "x", outline: [], z: 0, thickness: 0.2, note: CRASH_MARKER }] });
    const next = builder.build({ slabs: [{ id: "s", outline: [[0, 0], [1, 0], [1, 1]], z: 0, thickness: 0.2 }] });
    await assert.rejects(crash, (e) => e.code === "worker_failed");
    const { answer } = await next;
    assert.equal(answer.ok, true);
  } finally {
    builder.close();
  }
});

test("every answer names the version it was built from", async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    const { answer } = await builder.build({});
    assert.match(answer.goifcVersion, /^(v\d+\.\d+\.\d+.*|\(devel\))$/);
  } finally {
    builder.close();
  }
});

const slab = (id, extra = {}) => ({ slabs: [{ id, outline: [[0, 0], [1, 0], [1, 1]], z: 0, thickness: 0.2, ...extra }] });
const settle = (ms) => new Promise((r) => setTimeout(r, ms));

test("read refuses an include that is not an array of part names", { timeout: 20000 }, async () => {
  const reader = await createReader({ base: dist.href });
  try {
    const bytes = new Uint8Array(read(new URL("testdata/two_storey_spanning.ifc", root)));
    for (const include of ["psets", {}, new Set(["nodes"]), [() => 1], ["nodes", 1]]) {
      await assert.rejects(reader.read(bytes, { include }), (e) => e.code === "invalid_input", String(include));
    }
    const { answer } = await reader.read(bytes, { include: ["nodes"] });
    assert.deepEqual(answer.parts, ["nodes"]);
  } finally {
    reader.close();
  }
});

test("a request that cannot be sent rejects, and the next one works", { timeout: 20000 }, async () => {
  const builder = await createBuilder({ base: dist.href });
  try {
    await assert.rejects(builder.build(slab("x", { note: UNCLONABLE_MARKER })), (e) => e.name === "DataCloneError");
    const { answer } = await builder.build(slab("s"));
    assert.equal(answer.ok, true);
  } finally {
    builder.close();
  }
});

test("a retry after a dead worker starts one worker, not two", { timeout: 20000 }, async () => {
  const before = { ...workerCount };
  const builder = await createBuilder({ base: dist.href });
  const crash = builder.build(slab("x", { note: CRASH_MARKER }));
  const queued = builder.build(slab("q"));
  const retried = crash.catch(() => builder.build(slab("r")));
  assert.equal((await queued).answer.ok, true);
  assert.equal((await retried).answer.ok, true);
  assert.equal(workerCount.started - before.started, 2, "the first worker and one replacement");
  builder.close();
  await settle(200);
  assert.equal(workerCount.terminated - before.terminated, workerCount.started - before.started, "every worker terminated");
});

test("close() while a worker starts terminates it", { timeout: 20000 }, async () => {
  const before = { ...workerCount };
  const builder = await createBuilder({ base: dist.href });
  const crash = builder.build(slab("x", { note: CRASH_MARKER }));
  const queued = builder.build(slab("q"));
  await assert.rejects(crash);
  builder.close();
  await assert.rejects(queued, (e) => e.code === "worker_failed");
  await settle(1000);
  assert.equal(workerCount.terminated - before.terminated, workerCount.started - before.started, "every worker terminated");
});

test("base defaults to the folder of goifc.js, not the page", { timeout: 20000 }, async () => {
  const saved = Object.getOwnPropertyDescriptor(globalThis, "location");
  globalThis.location = { href: "file:///nowhere/app/page.html" };
  try {
    const { createBuilder: create } = await import(new URL("goifc.js", dist).href);
    const builder = await create();
    try {
      assert.equal((await builder.build(slab("s"))).answer.ok, true);
    } finally {
      builder.close();
    }
  } finally {
    if (saved) Object.defineProperty(globalThis, "location", saved);
    else Reflect.deleteProperty(globalThis, "location");
  }
});

test("reads resolve in call order when a Blob comes first", { timeout: 20000 }, async () => {
  const reader = await createReader({ base: dist.href });
  try {
    const bytes = new Uint8Array(read(new URL("testdata/two_storey_spanning.ifc", root)));
    const done = [];
    await Promise.all([
      reader.read(new Blob([bytes]), { include: ["nodes"] }).then(({ answer }) => done.push(answer.parts.join())),
      reader.read(bytes, { include: ["glb"] }).then(({ answer }) => done.push(answer.parts.join())),
    ]);
    assert.deepEqual(done, ["nodes", "glb"]);
  } finally {
    reader.close();
  }
});

test("close() terminates a replacement worker that never loads", { timeout: 20000 }, async () => {
  const before = { ...workerCount };
  const builder = await createBuilder({ base: dist.href });
  await assert.rejects(builder.build(slab("x", { note: CRASH_MARKER })));
  stall.next = true;
  const queued = builder.build(slab("q"));
  builder.close();
  await assert.rejects(queued, (e) => e.code === "worker_failed");
  assert.equal(workerCount.started - before.started, 2, "the first worker and the stalled replacement");
  assert.equal(workerCount.terminated - before.terminated, 2, "close() terminated the stalled worker");
});
