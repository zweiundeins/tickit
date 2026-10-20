// Builds a ProseMirror Schema from the shared JSON spec (internal/doc/schema.json).
// The browser adds toDOM and parseDOM per name; the oracle needs none.
import { Schema } from 'prosemirror-model'

export function buildSchema(spec, extra = {}) {
  const nodes = {}
  for (const { name, ...n } of spec.nodes) nodes[name] = { ...n, ...((extra.nodes && extra.nodes[name]) || {}) }
  const marks = {}
  for (const { name, ...m } of spec.marks) marks[name] = { ...m, ...((extra.marks && extra.marks[name]) || {}) }
  return new Schema({ nodes, marks, topNode: spec.topNode })
}
