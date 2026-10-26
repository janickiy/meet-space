import { readFile } from "node:fs/promises";

// Compare grouped shallow sizes, not dominator-tree retained sizes. Browser and
// debugger allocations appear alongside application objects in these snapshots.
const [beforePath, afterPath] = process.argv.slice(2);
if (!beforePath || !afterPath)
  throw new Error("Pass before and after Chromium .heapsnapshot files");

async function groups(filename) {
  const snapshot = JSON.parse(await readFile(filename, "utf8"));
  const fields = snapshot.snapshot.meta.node_fields;
  const typeOffset = fields.indexOf("type");
  const nameOffset = fields.indexOf("name");
  const sizeOffset = fields.indexOf("self_size");
  const types = snapshot.snapshot.meta.node_types[typeOffset];
  const result = new Map();
  for (let index = 0; index < snapshot.nodes.length; index += fields.length) {
    const type = types[snapshot.nodes[index + typeOffset]];
    const name = snapshot.strings[snapshot.nodes[index + nameOffset]];
    const key = `${type} ${name}`;
    const entry = result.get(key) || { type, name, count: 0, bytes: 0 };
    entry.count++;
    entry.bytes += snapshot.nodes[index + sizeOffset];
    result.set(key, entry);
  }
  return result;
}

const before = await groups(beforePath);
const after = await groups(afterPath);
const deltas = [...new Set([...before.keys(), ...after.keys()])]
  .map((key) => {
    const previous = before.get(key) || { count: 0, bytes: 0 };
    const current = after.get(key) || { count: 0, bytes: 0 };
    return {
      type: (after.get(key) || before.get(key)).type,
      name: (after.get(key) || before.get(key)).name,
      count: current.count - previous.count,
      bytes: current.bytes - previous.bytes,
    };
  })
  .sort((left, right) => right.bytes - left.bytes);
const typeDeltas = {};
for (const row of deltas) {
  const aggregate = (typeDeltas[row.type] ||= { count: 0, bytes: 0 });
  aggregate.count += row.count;
  aggregate.bytes += row.bytes;
}
console.log(
  JSON.stringify(
    {
      scope:
        "Grouped shallow-size differences; not retained-size dominators or proof of absence of leaks.",
      beforePath,
      afterPath,
      typeDeltas,
      largestIncreases: deltas.filter((row) => row.bytes > 0).slice(0, 25),
      largestDecreases: deltas.filter((row) => row.bytes < 0).slice(-10),
    },
    null,
    2,
  ),
);
