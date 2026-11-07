#!/usr/bin/env node
// Render the administrative and LDAP additions without touching existing diagrams.
const fs = require('node:fs');
const path = require('node:path');
const { instance } = require('@viz-js/viz');
const sharp = require('sharp');
async function main() {
  const root = path.resolve(__dirname, '../assets/technical-guide');
  const viz = await instance();
  for (const name of ['admin-architecture', 'ldap-lifecycle']) {
    const source = fs.readFileSync(path.join(root, `${name}.dot`), 'utf8');
    const svg = viz.renderString(source, { format: 'svg', engine: 'dot' });
    fs.writeFileSync(path.join(root, `${name}.svg`), svg);
    const output = path.join(root, `${name}.png`);
    // 120 dpi keeps the source crisp on A4 while bounding PDF image decoding.
    await sharp(Buffer.from(svg), { density: 120 }).flatten({ background: '#ffffff' }).png().toFile(output);
    const { width, height } = await sharp(output).metadata();
    console.log(JSON.stringify({name, width, height}));
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
