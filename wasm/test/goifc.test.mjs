// Runs goifc.js and the built .wasm files from dist/ (make wasm) under Node.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { CRASH_MARKER } from "./shims.mjs";
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
