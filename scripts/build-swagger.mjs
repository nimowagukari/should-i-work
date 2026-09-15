import { cp, mkdir } from "node:fs/promises";
import { getAbsoluteFSPath } from "swagger-ui-dist";

const src = getAbsoluteFSPath();
const dest = "docs";

await mkdir(dest, { recursive: true });

for (const file of [
  "swagger-ui.css",
  "swagger-ui-bundle.js",
  "swagger-ui-standalone-preset.js",
]) {
  await cp(`${src}/${file}`, `${dest}/${file}`);
}
