// docs/openapi.yaml が構文的に妥当な YAML かどうかを検証する。
// `npm run lint:openapi` から実行する。
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { parse } from "yaml";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const target = path.resolve(__dirname, "..", "docs", "openapi.yaml");

const text = readFileSync(target, "utf8");

let doc;
try {
  doc = parse(text);
} catch (err) {
  console.error(`YAML parse error in ${target}:`);
  console.error(err.message);
  process.exit(1);
}

if (!doc || typeof doc !== "object") {
  console.error(`${target} did not parse to an object`);
  process.exit(1);
}

if (!doc.openapi || !doc.paths) {
  console.error(`${target} is missing required top-level "openapi"/"paths" keys`);
  process.exit(1);
}

console.log(`OK: ${target} is valid YAML with ${Object.keys(doc.paths).length} path(s):`);
for (const [p, ops] of Object.entries(doc.paths)) {
  console.log(`  ${p} [${Object.keys(ops).join(", ")}]`);
}
