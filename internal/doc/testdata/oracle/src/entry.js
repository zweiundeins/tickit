// The oracle of internal/doc's differential tests: prosemirror-model and
// prosemirror-transform with the tickit schema, run in goja by go test.
// Every function takes and returns JSON strings, so no object crosses
// between goja and Go.
import { Node, Fragment } from 'prosemirror-model'
import { Step, StepMap, Mapping } from 'prosemirror-transform'
import { buildSchema } from './schema.js'
import { applyCommit } from './rebase.js'
import { fuzzCase, generator } from './gen.js'

let schema = null
const docs = new Map()
let nextId = 1

function doc(id) {
  const d = docs.get(id)
  if (!d) throw new RangeError('no document ' + id)
  return d
}

function mappingFrom(json) {
  const m = new Mapping()
  for (const [ranges, inverted] of json.maps) m.appendMap(new StepMap(ranges, !!inverted))
  for (let i = 0; i + 1 < (json.mirror || []).length; i += 2) m.setMirror(json.mirror[i], json.mirror[i + 1])
  return m
}

globalThis.oracle = {
  versions: { model: '1.25.12', transform: '1.12.2' },
  init(specJSON) {
    schema = buildSchema(JSON.parse(specJSON))
    return Object.keys(schema.nodes).length
  },
  // Node.fromJSON then toJSON: the document as prosemirror-model writes it.
  roundtrip(docJSON) {
    return JSON.stringify(Node.fromJSON(schema, JSON.parse(docJSON)).toJSON())
  },
  // fromJSON and check: "" when valid, else the error message.
  check(docJSON) {
    try {
      Node.fromJSON(schema, JSON.parse(docJSON)).check()
      return ''
    } catch (e) {
      return String(e.message)
    }
  },
  load(docJSON) {
    const id = nextId++
    docs.set(id, Node.fromJSON(schema, JSON.parse(docJSON)))
    return id
  },
  free(id) { docs.delete(id) },
  json(id) { return JSON.stringify(doc(id).toJSON()) },
  // Applies one step as Transform.maybeStep does: "" when it applied,
  // "failed: <reason>" when it failed (the doc stays), and throws otherwise.
  apply(id, stepJSON) {
    const step = Step.fromJSON(schema, JSON.parse(stepJSON))
    const res = step.apply(doc(id))
    if (res.failed) return 'failed: ' + res.failed
    docs.set(id, res.doc)
    return ''
  },
  // Rebases a commit's steps over `since` and applies them; returns the
  // steps as applied.
  rebase(id, sinceJSON, stepsJSON) {
    const since = JSON.parse(sinceJSON).map((j) => Step.fromJSON(schema, j))
    const steps = JSON.parse(stepsJSON).map((j) => Step.fromJSON(schema, j))
    const res = applyCommit(doc(id), since, steps)
    docs.set(id, res.doc)
    return JSON.stringify(res.steps.map((s) => s.toJSON()))
  },
  // Maps a step through a mapping given as {maps: [[ranges, inverted]], mirror: []}.
  mapStep(stepJSON, mappingJSON) {
    const mapped = Step.fromJSON(schema, JSON.parse(stepJSON)).map(mappingFrom(JSON.parse(mappingJSON)))
    return mapped ? JSON.stringify(mapped.toJSON()) : 'null'
  },
  mapPos(mappingJSON, pos, assoc) {
    const r = mappingFrom(JSON.parse(mappingJSON)).mapResult(pos, assoc)
    return JSON.stringify({ pos: r.pos, deleted: r.deleted, before: r.deletedBefore, after: r.deletedAfter, across: r.deletedAcross })
  },
  stepMap(stepJSON) {
    const m = Step.fromJSON(schema, JSON.parse(stepJSON)).getMap()
    return JSON.stringify({ ranges: m.ranges, inverted: m.inverted })
  },
  diffStart(aJSON, bJSON) {
    const a = Node.fromJSON(schema, JSON.parse(aJSON)), b = Node.fromJSON(schema, JSON.parse(bJSON))
    return JSON.stringify(a.content.findDiffStart(b.content))
  },
  diffEnd(aJSON, bJSON) {
    const a = Node.fromJSON(schema, JSON.parse(aJSON)), b = Node.fromJSON(schema, JSON.parse(bJSON))
    return JSON.stringify(a.content.findDiffEnd(b.content))
  },
  textBetween(docJSON, sep, leaf) {
    const d = Node.fromJSON(schema, JSON.parse(docJSON))
    return d.textBetween(0, d.content.size, sep, leaf)
  },
  // A document of about kb kilobytes made by the fixture generator.
  genDoc(seed, kb) { return JSON.stringify(generator(schema, seed).doc(kb).toJSON()) },
  // A fuzz case for FuzzRebase: {doc, commits: [{base, steps, applied | error}], final}.
  fuzzCase(seed, optsJSON) { return JSON.stringify(fuzzCase(schema, seed, JSON.parse(optsJSON))) },
}
