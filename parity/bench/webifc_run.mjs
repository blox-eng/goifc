// web-ifc runner for the reader benchmark: same stages, same record as goifc/main.go.
import { readFileSync, writeFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import * as WebIFC from "web-ifc";

const args = process.argv.slice(2);
const boxesAt = args.indexOf("--boxes");
const boxesPath = boxesAt >= 0 ? args.splice(boxesAt, 2)[1] : null;
const [modelPath] = args;

const peakRSSMiB = () => {
  const m = readFileSync("/proc/self/status", "utf8").match(/^VmHWM:\s+(\d+) kB/m);
  return m ? Number(m[1]) / 1024 : 0;
};

const api = new WebIFC.IfcAPI();
await api.Init(undefined, true);
const data = readFileSync(modelPath);
const rec = { tool: "web-ifc" };

let t = performance.now();
const id = api.OpenModel(data);
rec.parse_ms = performance.now() - t;

// Walk: what a quantity reader needs per product — identity, property and
// quantity sets, and the spatial container. The helpers in api.properties
// rescan every relationship per element, so this resolves each relationship
// once instead, which is how a performance-minded caller would write it.
t = performance.now();
const psetsOf = new Map();
for (const relID of api.GetLineIDsWithType(id, WebIFC.IFCRELDEFINESBYPROPERTIES)) {
  const rel = api.GetLine(id, relID);
  const def = api.GetLine(id, rel.RelatingPropertyDefinition.value, true);
  for (const o of rel.RelatedObjects) {
    const list = psetsOf.get(o.value) ?? [];
    list.push(def);
    psetsOf.set(o.value, list);
  }
}
const containerOf = new Map();
for (const relID of api.GetLineIDsWithType(id, WebIFC.IFCRELCONTAINEDINSPATIALSTRUCTURE)) {
  const rel = api.GetLine(id, relID);
  for (const e of rel.RelatedElements) containerOf.set(e.value, rel.RelatingStructure.value);
}
let products = 0;
const guidOf = new Map();
for (const pid of api.GetLineIDsWithType(id, WebIFC.IFCPRODUCT, true)) {
  const p = api.GetLine(id, pid);
  guidOf.set(pid, p.GlobalId?.value);
  void p.Name, psetsOf.get(pid), containerOf.get(pid);
  products++;
}
rec.walk_ms = performance.now() - t;
rec.products = products;

// Tessellate, and read each vertex buffer out of the heap as a caller must.
t = performance.now();
const meshes = [];
api.StreamAllMeshes(id, (mesh) => {
  const parts = [];
  for (let i = 0; i < mesh.geometries.size(); i++) {
    const pg = mesh.geometries.get(i);
    const g = api.GetGeometry(id, pg.geometryExpressID);
    const v = api.GetVertexArray(g.GetVertexData(), g.GetVertexDataSize()).slice();
    parts.push({ v, m: pg.flatTransformation.slice() });
    g.delete();
  }
  meshes.push({ id: mesh.expressID, parts });
});
rec.geom_ms = performance.now() - t;

// web-ifc emits Y-up; convert back to IFC's Z-up world (x, y, z) = (X, -Z, Y).
const boxes = {};
for (const { id: eid, parts } of meshes) {
  const lo = [Infinity, Infinity, Infinity], hi = [-Infinity, -Infinity, -Infinity];
  for (const { v, m } of parts) {
    for (let i = 0; i < v.length; i += 6) {
      const x = v[i], y = v[i + 1], z = v[i + 2];
      const X = m[0] * x + m[4] * y + m[8] * z + m[12];
      const Y = m[1] * x + m[5] * y + m[9] * z + m[13];
      const Z = m[2] * x + m[6] * y + m[10] * z + m[14];
      const w = [X, -Z, Y];
      for (let k = 0; k < 3; k++) { lo[k] = Math.min(lo[k], w[k]); hi[k] = Math.max(hi[k], w[k]); }
    }
  }
  const guid = guidOf.get(eid);
  if (guid && Number.isFinite(lo[0])) boxes[guid] = { min: lo, max: hi };
}
rec.meshes = Object.keys(boxes).length;
rec.peak_rss_mib = peakRSSMiB();

if (boxesPath) writeFileSync(boxesPath, JSON.stringify(boxes));
api.CloseModel(id);
console.log(JSON.stringify(rec));
