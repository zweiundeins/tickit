// Generators for the differential tests: documents in the tickit schema, and
// edits made by prosemirror-transform the way an editor makes them. Node
// (js/gen-fixtures.mjs) writes the fixtures with them; the goja oracle runs
// them inside go test for FuzzRebase. Positions never fall inside a
// surrogate pair, as a browser never puts a caret there.
import { Fragment, Slice } from 'prosemirror-model'
import { Transform, liftTarget, canJoin, canSplit, findWrapping } from 'prosemirror-transform'
import { applyCommit } from './rebase.js'

// mulberry32: a small seeded generator that gives the same numbers in V8 and goja.
export function rng(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const words = 'the quick brown fox jumps over lazy dog release plan sprint issue page editor server writer step version cut fold table list task review deploy schema'.split(' ')
const people = ['s-ada', 's-grace', 's-linus', 's-sunny-otter', 'g-mfrggzdfmztwq2lknnwg', 'a-0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d']
const languages = ['', 'go', 'js', 'sql', 'text', 'bash']
const typed = Array.from('abcdefghijklmnopqrstuvwxyz  .,é😀ß')

class Gen {
  constructor(schema, rnd) {
    this.s = schema
    this.n = schema.nodes
    this.m = schema.marks
    this.rnd = rnd
  }
  int(n) { return Math.floor(this.rnd() * n) }
  pick(a) { return a[this.int(a.length)] }
  sentence(n) { return Array.from({ length: n }, () => this.pick(words)).join(' ') }
  hex(n) { let s = ''; for (let i = 0; i < n; i++) s += '0123456789abcdef'[this.int(16)]; return s }
  pageId() { return this.hex(8) + '-' + this.hex(4) + '-7' + this.hex(3) + '-' + this.pick(['8', '9', 'a', 'b']) + this.hex(3) + '-' + this.hex(12) }
  chip() { return this.n.issue_chip.create({ key: this.pick(['WEB', 'API', 'OPS']) + '-' + (1 + this.int(300)) }) }
  mention() { return this.n.mention.create({ personId: this.pick(people), label: this.pick(['Ada', 'Grace Hopper', 'Linus', 'Sunny Otter']) }) }
  pageLink() { return this.n.page_link.create({ pageId: this.pageId() }) }
  link() { return this.m.link.create({ href: this.rnd() < 0.8 ? 'https://example.com/' + this.pick(words) : 'mailto:ada@example.com' }) }

  inline() {
    const { s, m } = this
    const parts = []
    const k = 3 + this.int(6)
    for (let i = 0; i < k; i++) {
      const r = this.rnd()
      if (r < 0.55) parts.push(s.text(this.sentence(3 + this.int(8)) + ' '))
      else if (r < 0.65) parts.push(s.text(this.sentence(2) + ' ', [m.strong.create()]))
      else if (r < 0.72) parts.push(s.text(this.sentence(2) + ' ', [m.em.create()]))
      else if (r < 0.76) parts.push(s.text(this.pick(words), [m.code.create()]), s.text(' '))
      else if (r < 0.82) parts.push(s.text(this.sentence(2), [this.link()]), s.text(' '))
      else if (r < 0.88) parts.push(this.chip(), s.text(' '))
      else if (r < 0.93) parts.push(this.mention(), s.text(' '))
      else if (r < 0.96) parts.push(this.pageLink(), s.text(' '))
      else parts.push(this.n.hard_break.create())
    }
    return parts
  }
  para() { return this.n.paragraph.create(null, this.inline()) }
  item(t) { return t.create(t === this.n.task_item ? { checked: this.rnd() < 0.5 } : null, [this.para()]) }
  cell(h) { return (h ? this.n.table_header : this.n.table_cell).create(null, [this.n.paragraph.create(null, [this.s.text(this.sentence(2))])]) }
  table(rows, cols) { return this.n.table.create(null, Array.from({ length: rows }, (_, r) => this.n.table_row.create(null, Array.from({ length: cols }, () => this.cell(r === 0))))) }
  code() { return this.n.code_block.create({ language: this.pick(languages) }, [this.s.text('func main() {\n\tfmt.Println("' + this.sentence(3) + '")\n}')]) }
  image() { return this.n.image.create({ imageId: 'img-' + this.int(40), alt: this.sentence(3) }) }

  section() {
    const n = this.n
    const b = [n.heading.create({ level: 1 + this.int(3) }, [this.s.text(this.sentence(4))])]
    b.push(this.para(), this.para())
    b.push(n.bullet_list.create(null, [this.item(n.list_item), this.item(n.list_item), this.item(n.list_item)]))
    b.push(n.task_list.create(null, [this.item(n.task_item), this.item(n.task_item)]))
    if (this.rnd() < 0.5) b.push(n.ordered_list.create({ order: this.rnd() < 0.8 ? 1 : 3 }, [this.item(n.list_item), this.item(n.list_item)]))
    if (this.rnd() < 0.4) b.push(this.code())
    if (this.rnd() < 0.4) b.push(n.blockquote.create(null, [this.para()]))
    if (this.rnd() < 0.3) b.push(this.table(3, 4))
    if (this.rnd() < 0.2) b.push(n.horizontal_rule.create())
    if (this.rnd() < 0.1) b.push(this.image())
    return b
  }

  // A document of about kb kilobytes of JSON.
  doc(kb) {
    const blocks = []
    let doc
    do {
      blocks.push(...this.section())
      doc = this.n.doc.create(null, blocks)
    } while (JSON.stringify(doc.toJSON()).length < kb * 1024)
    doc.check()
    return doc
  }

  textblocks(d) {
    const out = []
    d.descendants((node, pos) => {
      if (node.isTextblock) { out.push({ node, start: pos + 1, end: pos + 1 + node.content.size }); return false }
      return true
    })
    return out
  }

  // A position inside a surrogate pair moves to the pair's start.
  snap(d, pos) {
    const $p = d.resolve(pos)
    if (!$p.textOffset) return pos
    const t = $p.parent.child($p.index()).text
    const c = t.charCodeAt($p.textOffset - 1)
    return c >= 0xd800 && c <= 0xdbff ? pos - 1 : pos
  }
  anyPos(d) { return this.snap(d, this.int(d.content.size + 1)) }
  textPos(d) { const t = this.pick(this.textblocks(d)); return { t, pos: this.snap(d, t.start + this.int(t.end - t.start + 1)) } }

  // One edit of the batch fixtures (typing, deletes, marks, splits, joins,
  // block types, attrs, wraps, lifts, pastes, chips). Returns a Transform or null.
  batchEdit(d, noAttr) {
    const { s, n, m } = this
    const tr = new Transform(d)
    const r = this.rnd()
    const { t, pos } = this.textPos(d)
    const code = t.node.type === n.code_block
    try {
      if (r < 0.6) tr.replaceWith(pos, pos, s.text(this.pick(typed), d.resolve(pos).marks()))
      else if (r < 0.7) { if (pos < t.end) { let e = this.snap(d, pos + 1); if (e === pos) e = Math.min(pos + 2, t.end); tr.delete(pos, e) } }
      else if (r < 0.76) { if (!code && t.end - t.start > 4) { const a = this.snap(d, t.start + this.int(t.end - t.start - 3)); tr.addMark(a, this.snap(d, a + 3), this.pick([m.strong, m.em, m.strike]).create()) } }
      else if (r < 0.8) { if (!code) tr.removeMark(t.start, t.end, this.pick([m.strong, m.em])) }
      else if (r < 0.85) { if (!code && t.node.type === n.paragraph) tr.split(pos) }
      else if (r < 0.89) {
        const $e = d.resolve(t.end + 1)
        if (t.node.type === n.paragraph && $e.nodeAfter && $e.nodeAfter.type === n.paragraph && canJoin(d, t.end + 1)) tr.join(t.end + 1)
      } else if (r < 0.92) {
        const $p = d.resolve(t.start)
        if (t.node.type === n.heading) tr.setNodeMarkup(t.start - 1, null, { level: 1 + this.int(3) })
        else if (t.node.type === n.paragraph && $p.parent.canReplaceWith($p.index(), $p.index() + 1, n.heading)) tr.setBlockType(t.start, t.start, n.heading, { level: 2 })
      } else if (r < 0.95) {
        if (noAttr) return null
        const items = []
        d.descendants((node, p) => { if (node.type === n.task_item || node.type === n.code_block) items.push({ node, p }) })
        if (items.length) {
          const it = this.pick(items)
          if (it.node.type === n.task_item) tr.setNodeAttribute(it.p, 'checked', !it.node.attrs.checked)
          else tr.setNodeAttribute(it.p, 'language', this.pick(languages))
        }
      } else if (r < 0.97) {
        const $f = d.resolve(t.start), range = $f.blockRange()
        if (range && $f.depth === 1 && t.node.type === n.paragraph) tr.wrap(range, [{ type: n.blockquote }])
        else if (range) { const tgt = liftTarget(range); if (tgt != null) tr.lift(range, tgt) }
      } else if (r < 0.99) { if (!code) tr.replaceWith(pos, pos, s.text('pasted ' + this.pick(words) + ' ', [m.strong.create()])) }
      else { if (!code) tr.insert(pos, this.chip()) }
    } catch (e) { return null }
    return tr.steps.length ? tr : null
  }

  // The steps of one commit of the commit fixtures: five edits (typing,
  // deletes, bold over a textblock) on the version the client sees.
  commitSteps(doc) {
    const tbs = this.textblocks(doc).map((t) => [t.start, t.end])
    const tr = new Transform(doc)
    for (let i = 0; i < 5; i++) {
      const [a, b] = this.pick(tbs)
      const p = this.snap(tr.doc, tr.mapping.map(a + this.int(b - a + 1)))
      const r = this.rnd()
      if (r < 0.75) tr.replaceWith(p, p, this.s.text(this.pick(['x', 'y', 'z', 'é', '😀']), tr.doc.resolve(p).marks()))
      else if (r < 0.9 && p < tr.mapping.map(b)) { let e = this.snap(tr.doc, p + 1); if (e === p) e = p + 2; if (e <= tr.mapping.map(b)) tr.delete(p, e) }
      else tr.addMark(tr.mapping.map(a), tr.mapping.map(b), this.m.strong.create())
    }
    return tr.steps
  }

  // One edit of the fuzz commits: everything an editor does, including
  // deletes across blocks, pastes of open slices, table edits and attrs.
  fuzzEdit(tr) {
    const { s, n, m } = this
    const d = tr.doc
    const r = this.int(20)
    const { t, pos } = this.textPos(d)
    const code = t.node.type === n.code_block
    switch (r) {
      case 0: case 1: case 2: case 3:
        tr.replaceWith(pos, pos, s.text(this.pick(typed), d.resolve(pos).marks())); break
      case 4: { // delete inside a textblock
        if (pos < t.end) tr.delete(pos, this.snap(d, Math.min(t.end, pos + 1 + this.int(6))))
        break
      }
      case 5: { // delete across blocks
        let a = this.anyPos(d), b = this.anyPos(d)
        if (a > b) [a, b] = [b, a]
        if (b - a > 200) b = this.snap(d, a + 200)
        tr.delete(a, b); break
      }
      case 6: { // delete a whole block, often the one another editor types in
        const $p = d.resolve(pos)
        const depth = 1 + this.int($p.depth)
        tr.delete($p.before(depth), $p.after(depth)); break
      }
      case 7: { // add a mark over any range
        let a = this.anyPos(d), b = this.anyPos(d)
        if (a > b) [a, b] = [b, a]
        tr.addMark(a, b, this.rnd() < 0.2 ? this.link() : this.pick([m.strong, m.em, m.strike, m.code]).create()); break
      }
      case 8: { // remove a mark over any range
        let a = this.anyPos(d), b = this.anyPos(d)
        if (a > b) [a, b] = [b, a]
        tr.removeMark(a, b, this.pick([m.strong, m.em, m.link, null])); break
      }
      case 9: if (!code && canSplit(d, pos)) tr.split(pos); break
      case 10: { // join two blocks
        const p = this.anyPos(d)
        if (canJoin(d, p)) tr.join(p)
        break
      }
      case 11: { // block type
        const $p = d.resolve(t.start)
        const type = this.pick([n.heading, n.paragraph, n.code_block])
        const attrs = type === n.heading ? { level: 1 + this.int(3) } : type === n.code_block ? { language: this.pick(languages) } : null
        if ($p.parent.canReplaceWith($p.index(), $p.index() + 1, type)) tr.setBlockType(t.start, t.end, type, attrs)
        break
      }
      case 12: { // attrs
        const items = []
        d.descendants((node, p) => { if (!node.isText && Object.keys(node.attrs).length) items.push({ node, p }) })
        if (!items.length) break
        const { node, p } = this.pick(items)
        const name = node.type.name
        if (name === 'task_item') tr.setNodeAttribute(p, 'checked', !node.attrs.checked)
        else if (name === 'heading') tr.setNodeAttribute(p, 'level', 1 + this.int(3))
        else if (name === 'code_block') tr.setNodeAttribute(p, 'language', this.pick(languages))
        else if (name === 'ordered_list') tr.setNodeAttribute(p, 'order', 1 + this.int(9))
        else if (name === 'table_cell' || name === 'table_header') tr.setNodeAttribute(p, 'colwidth', this.rnd() < 0.5 ? null : [100 + this.int(100)])
        else if (name === 'image') tr.setNodeAttribute(p, 'alt', this.sentence(2))
        else if (name === 'mention') tr.setNodeAttribute(p, 'label', 'Ada')
        else if (name === 'issue_chip') tr.setNodeAttribute(p, 'key', 'WEB-' + (1 + this.int(300)))
        break
      }
      case 13: { // wrap or lift
        const $f = d.resolve(t.start), range = $f.blockRange()
        if (!range) break
        if (this.rnd() < 0.5) {
          const wrapping = findWrapping(range, this.pick([n.blockquote, n.bullet_list, n.task_list, n.ordered_list]))
          if (wrapping) tr.wrap(range, wrapping)
        } else {
          const tgt = liftTarget(range)
          if (tgt != null) tr.lift(range, tgt)
        }
        break
      }
      case 14: { // paste a slice of the document somewhere else
        let a = this.anyPos(d), b = this.anyPos(d)
        if (a > b) [a, b] = [b, a]
        if (b - a > 120) b = this.snap(d, a + 120)
        const slice = d.slice(a, b)
        let c = this.anyPos(d), e = this.rnd() < 0.7 ? c : this.anyPos(d)
        if (c > e) [c, e] = [e, c]
        tr.replace(c, e, slice); break
      }
      case 15: { // inline atoms
        if (code) break
        tr.insert(pos, this.pick([() => this.chip(), () => this.mention(), () => this.pageLink(), () => n.hard_break.create()])())
        break
      }
      case 16: { // a block at a block boundary
        const $p = d.resolve(pos)
        const at = $p.after(1)
        const node = this.pick([() => n.horizontal_rule.create(), () => this.image(), () => this.table(1 + this.int(2), 1 + this.int(3)), () => this.code(), () => this.para()])()
        tr.insert(at, node); break
      }
      case 17: { // table rows: add a copy of a row or delete one
        const rows = []
        d.descendants((node, p) => { if (node.type === n.table_row) rows.push({ node, p }); return node.type !== n.table_row })
        if (!rows.length) break
        const { node, p } = this.pick(rows)
        if (this.rnd() < 0.6) tr.insert(p + node.nodeSize, node.copy(Fragment.from(node.content.content.map((c) => c.type.createAndFill(c.attrs)))))
        else tr.delete(p, p + node.nodeSize)
        break
      }
      case 18: { // select a range in a textblock and type over it
        let e = this.snap(d, Math.min(t.end, pos + this.int(8)))
        tr.replaceWith(pos, e, s.text(this.pick(words), d.resolve(pos).marks())); break
      }
      case 19: { // replace a range with an open slice made by hand
        let a = this.anyPos(d), b = this.anyPos(d)
        if (a > b) [a, b] = [b, a]
        if (b - a > 60) b = this.snap(d, a + 60)
        const frag = Fragment.from([n.paragraph.create(null, s.text(this.sentence(2))), n.paragraph.create(null, s.text(this.sentence(1)))])
        tr.replace(a, b, new Slice(frag, 1, 1)); break
      }
    }
  }

  fuzzSteps(doc, edits) {
    const tr = new Transform(doc)
    for (let i = 0; i < edits; i++) {
      try { this.fuzzEdit(tr) } catch (e) { /* a helper refused this edit */ }
    }
    return tr.steps
  }
}

export function generator(schema, seed) { return new Gen(schema, rng(seed)) }

// A fuzz case: a small document and commits, each made on a version up to
// maxK commits behind the head and rebased by applyCommit, as the writer does.
export function fuzzCase(schema, seed, opts) {
  const g = generator(schema, seed)
  const kb = opts.kb || 2, commits = opts.commits || 12, maxK = opts.maxK == null ? 6 : opts.maxK, edits = opts.edits || 4
  let doc = g.doc(kb)
  const start = doc
  const docs = [doc], history = []
  const out = []
  for (let i = 0; i < commits; i++) {
    const head = history.length
    const base = Math.max(0, head - g.int(maxK + 1))
    const steps = g.fuzzSteps(docs[base], 1 + g.int(edits))
    let res
    try {
      res = applyCommit(doc, history.slice(base).flat(), steps)
    } catch (e) {
      // The writer refuses the whole commit; the head stays.
      out.push({ base, steps: steps.map((s) => s.toJSON()), error: String(e.message) })
      continue
    }
    out.push({ base, steps: steps.map((s) => s.toJSON()), applied: res.steps.map((s) => s.toJSON()) })
    history.push(res.steps)
    doc = res.doc
    docs.push(doc)
  }
  return { doc: start.toJSON(), commits: out, final: doc.toJSON() }
}
