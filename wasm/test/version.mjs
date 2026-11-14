// Prints the goifcVersion that dist/goifc-build.wasm answers with. The
// release job compares it with the tag it built.
import "./shims.mjs";
import { createBuilder } from "../goifc.js";

const builder = await createBuilder({ base: new URL("../../dist/", import.meta.url).href });
const { answer } = await builder.build({});
builder.close();
console.log(answer.goifcVersion);
