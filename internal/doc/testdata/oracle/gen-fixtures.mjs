// Writes the fixtures of internal/doc's differential tests with ProseMirror
// in V8 (Node). Usage, from this folder: node gen-fixtures.mjs
//
// For each document size: <name>-doc.json (a document of about that many KB),
// <name>-batches.json (200 batches of 5 steps made by prosemirror-transform),
// <name>-final.json (the document after them), and for each k
// <name>-k<k>-commits.json (300 commits, each made on the version k commits
// behind the head, the steps as the client made them) with
// <name>-k<k>-final.json (the document after the writer rebased them all).
import { readFileSync, writeFileSync } from 'node:fs'
import { Node } from 'prosemirror-model'
import { buildSchema } from './src/schema.js'
import { applyCommit } from './src/rebase.js'
import { generator } from './src/gen.js'

const here = new URL('.', import.meta.url)
const out = new URL('../', here)
const schema = buildSchema(JSON.parse(readFileSync(new URL('../../schema.json', here))))
const write = (name, value) => writeFileSync(new URL(name, out), JSON.stringify(value))

function batches(name, kb, n, seed) {
  const g = generator(schema, seed)
  let doc = g.doc(kb)
  const start = doc
  const list = []
  const kinds = {}
  while (list.length < n) {
    const batch = []
    while (batch.length < 5) {
      const tr = g.batchEdit(doc)
      if (!tr) continue
      for (const st of tr.steps) {
        if (batch.length === 5) break
        const j = st.toJSON()
        kinds[j.stepType] = (kinds[j.stepType] || 0) + 1
        batch.push(j)
        doc = st.apply(doc).doc
      }
    }
    list.push(batch)
  }
  doc.check()
  write(name + '-doc.json', start.toJSON())
  write(name + '-batches.json', list)
  write(name + '-final.json', doc.toJSON())
  console.log(JSON.stringify({ name, kb: +(JSON.stringify(start.toJSON()).length / 1024).toFixed(1), blocks: start.childCount, batches: n, kinds }))
}

function commits(name, k, n, seed) {
  const g = generator(schema, seed)
  let doc = Node.fromJSON(schema, JSON.parse(readFileSync(new URL(name + '-doc.json', out))))
  const docs = [doc], history = [], list = []
  let dropped = 0
  for (let i = 0; i < n; i++) {
    const base = Math.max(0, history.length - k)
    const steps = g.commitSteps(docs[base])
    const res = applyCommit(doc, history.slice(base).flat(), steps)
    dropped += steps.length - res.steps.length
    list.push({ base, steps: steps.map((s) => s.toJSON()) })
    history.push(res.steps)
    doc = res.doc
    docs.push(doc)
  }
  doc.check()
  write(`${name}-k${k}-commits.json`, list)
  write(`${name}-k${k}-final.json`, doc.toJSON())
  console.log(JSON.stringify({ name, k, commits: n, steps: list.reduce((a, c) => a + c.steps.length, 0), dropped }))
}

batches('f20k', 20, 200, 42)
batches('f200k', 200, 200, 4242)
for (const k of [0, 1, 5, 20]) commits('f20k', k, 300, 7 + k)
commits('f200k', 5, 300, 77)
