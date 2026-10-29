#!/usr/bin/env node
// Render editable Graphviz sources; Graphviz does layout, sharp rasterizes SVG.
// Run with @viz-js/viz and sharp available through Node's normal module lookup.
const fs = require('node:fs');
const path = require('node:path');
const { instance } = require('@viz-js/viz');
const sharp = require('sharp');

async function main() {
  const root = path.resolve(__dirname, '../assets/technical-guide');
  const viz = await instance();
  for (const name of ['architecture', 'architecture-workers', 'media-flow', 'er-01-identity-overview']) {
    const source = fs.readFileSync(path.join(root, `${name}.dot`), 'utf8');
    const svg = viz.renderString(source, { format: 'svg', engine: 'dot' });
    fs.writeFileSync(path.join(root, `${name}.svg`), svg);
    await sharp(Buffer.from(svg), { density: 180 }).flatten({ background: '#ffffff' })
      .png().toFile(path.join(root, `${name}.png`));
    console.log(`Rendered ${name}`);
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
