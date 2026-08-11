#!/usr/bin/env node
// render_box.mjs — visual proof harness for the tokenize->semantic->GLB pipeline.
//
// Loads a .glb (produced by internal/boxglb.WriteBox) into a headless
// three.js scene via Playwright, orbits a camera around the mesh's bounding
// box, and screenshots the result to a PNG. THROWAWAY — this only exists to
// visually confirm the tokenize→semantic→GLB→render pipeline shape;
// it is not the real geometry viewer.
//
// Usage: node render_box.mjs <in.glb> <out.png>
//
// Requires `playwright` (for the headless Chromium browser) on the PATH /
// in node_modules. three.js is loaded from a CDN via an import map so no
// local three.js install is required.

import { chromium } from "playwright";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const [, , inPath, outPath] = process.argv;
if (!inPath || !outPath) {
	console.error("usage: node render_box.mjs <in.glb> <out.png>");
	process.exit(1);
}

const glbBytes = readFileSync(resolve(inPath));
const glbBase64 = glbBytes.toString("base64");

const html = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>html,body{margin:0;background:#1a1a1a;}canvas{display:block;}</style>
<script type="importmap">
{
  "imports": {
    "three": "https://unpkg.com/three@0.160.0/build/three.module.js",
    "three/addons/": "https://unpkg.com/three@0.160.0/examples/jsm/"
  }
}
</script>
</head>
<body>
<script type="module">
import * as THREE from "three";
import { GLTFLoader } from "three/addons/loaders/GLTFLoader.js";

const WIDTH = 800, HEIGHT = 600;

const scene = new THREE.Scene();
scene.background = new THREE.Color(0x1a1a1a);

const camera = new THREE.PerspectiveCamera(50, WIDTH / HEIGHT, 0.01, 1000);

const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setSize(WIDTH, HEIGHT);
document.body.appendChild(renderer.domElement);

scene.add(new THREE.AmbientLight(0xffffff, 0.6));
const dir = new THREE.DirectionalLight(0xffffff, 0.8);
dir.position.set(5, 10, 7);
scene.add(dir);

function base64ToArrayBuffer(b64) {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

const glbBuffer = base64ToArrayBuffer("${glbBase64}");

window.__renderDone = false;
window.__renderError = null;

const loader = new GLTFLoader();
loader.parse(glbBuffer, "", (gltf) => {
  const mesh = gltf.scene;
  mesh.traverse((o) => {
    if (o.isMesh) {
      o.material = new THREE.MeshStandardMaterial({ color: 0x8899aa });
    }
  });
  scene.add(mesh);

  const box = new THREE.Box3().setFromObject(mesh);
  const center = box.getCenter(new THREE.Vector3());
  const size = box.getSize(new THREE.Vector3());
  const maxDim = Math.max(size.x, size.y, size.z, 0.1);

  camera.position.set(
    center.x + maxDim * 1.5,
    center.y + maxDim * 1.2,
    center.z + maxDim * 1.5,
  );
  camera.lookAt(center);
  camera.updateProjectionMatrix();

  renderer.render(scene, camera);
  window.__renderDone = true;
}, (err) => {
  window.__renderError = String(err);
});
</script>
</body>
</html>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 800, height: 600 } });
await page.setContent(html);
await page.waitForFunction(
	() => window.__renderDone === true || window.__renderError !== null,
	{ timeout: 15000 },
);
const renderError = await page.evaluate(() => window.__renderError);
if (renderError) {
	console.error("render failed:", renderError);
	await browser.close();
	process.exit(1);
}

const pngBuffer = await page.screenshot();
writeFileSync(resolve(outPath), pngBuffer);
await browser.close();

console.log(`wrote ${outPath} (${pngBuffer.length} bytes)`);
