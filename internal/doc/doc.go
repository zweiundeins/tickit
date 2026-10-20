// Package doc is the rich-text document of tickit's pages and descriptions:
// a Go port of the parts of prosemirror-model and prosemirror-transform the
// server runs, for the schema in schema.json, with the server's rules, the
// HTML renderer, search text, references, version diffs and a Markdown
// export. It has no dependency on lith; documents are immutable and can be
// shared between goroutines.
//
// # What is ported
//
// From prosemirror-model 1.25.12: content expressions (parsed and compiled
// to the same automaton, edges in the same order), the schema with
// attribute defaults and validation (the "validate" type strings), mark
// sets and their order, Fragment, Node, Slice, ResolvedPos, the replace
// algorithm with its checks, check, toJSON and fromJSON, findDiffStart and
// findDiffEnd. From prosemirror-transform 1.12.2: the steps replace,
// replaceAround, addMark, removeMark and attr (fromJSON, toJSON, apply,
// getMap, map), StepMap and Mapping with mirrors, invert and slice. Rebase
// is applyCommit of @stepwisehq/prosemirror-collab-commit, as the writer
// runs it. The Transform helpers (lift, wrap, split, the Fitter), invert
// and merge of steps and the DOM code are not ported: the browser makes
// the steps, the server applies and rebases them.
//
// JSON goes in and out as prosemirror-model reads and writes it, with
// JavaScript's semantics where they show: key order (attributes in schema
// order, then the keys of nested objects as JSON.stringify writes them),
// numbers as Number.prototype.toString writes them, strings escaped as
// JSON.stringify escapes them (not <, > or &, which encoding/json would),
// truthiness of missing or null attributes, adjacent text nodes with the
// same marks joined. A document parses and writes back byte for byte as
// the browser writes it, and errors carry prosemirror-model's messages.
//
// # Positions
//
// Text is kept as UTF-8, but sizes and positions count UTF-16 code units,
// as ProseMirror's positions do in the browser: an emoji outside the Basic
// Multilingual Plane has size 2. A JSON string with half of a surrogate
// pair is refused (ErrLoneSurrogate), and so is a step that would cut a
// pair in half (ErrSurrogateSplit): ProseMirror would keep the half, Go's
// strings cannot, and browsers never put a caret inside a pair.
//
// # What is refused
//
// ParseSteps reads only the five step types; addNodeMark, removeNodeMark,
// docAttr and every other type are refused (ErrStepType). It also refuses
// steps the browser never makes, which prosemirror-transform would accept
// and then misplace or throw on (ErrBadStep): positions that are not whole
// numbers from 0, a range that ends before it starts, a replaceAround gap
// outside its range, a slice open deeper than its content. Some malformed
// JSON that JavaScript would read by accident is refused too: a value
// overwritten by a duplicate key still has to be valid, and numbers must
// fit a float64. ParseCommit adds the limits of a commit: 500 steps and
// 256 KB.
//
// Step.Apply returns a *StepFailed error where prosemirror-transform
// returns a failed StepResult (the step does not fit; Rebase drops it, as
// the browser does), and another error where it throws (Rebase refuses the
// whole commit).
//
// # Server rules
//
// Rules.Check runs after each commit on the top-level blocks the commit
// changed (an unchanged block keeps its node, so a pointer comparison
// finds the changed ones), and refuses the commit with a sentence a person
// can read (a *RuleError). The changed blocks pass the schema's check,
// which Step.Apply does not do for content inside a slice. The limits come
// from schema.json:
//
//   - The document: 200 KB of stored JSON and 2,000 block nodes (DM-05.1),
//     refused only when a commit grows a document past them. JSON length
//     and block counts are cached in each node, so the check costs the
//     changed blocks only.
//   - Text: no control characters but tab and newline in code blocks.
//   - heading.level 1, 2 or 3; code_block.language empty or one of
//     codeLanguages; ordered_list.order a whole number from 1 to 9,999.
//   - Tables (FR-83): 50 rows, 20 columns counted with colspans and the
//     rowspans of the rows above; colspan 1 to 20, rowspan 1 to 50;
//     colwidth null or one number from 0 to 10,000 per spanned column.
//   - image (FR-84): imageId in the library (the Rules.Image callback);
//     src empty or /img/lib/<imageId>; alt at most 500 characters.
//   - issue_chip.key matches ^[A-Z]{2,10}-[1-9][0-9]{0,6}$ (FR-100).
//   - mention.personId is a person id (g- and 20 characters of base32, a-
//     and a UUIDv7, s- and a slug); its label is empty or 2 to 30
//     characters.
//   - page_link.pageId is a UUIDv7 (FR-103).
//   - link.href passes SafeHref: http, https or mailto, parsed by net/url,
//     with a host for http and https, at most 2,048 characters (DM-06.2).
//   - Every attribute string holds at most attrLength (2,048) characters.
//
// # Rendering
//
// RenderHTML produces the HTML table of docs/editor.md. It escapes every
// text and attribute, renders links with rel="nofollow ugc noopener" and
// their host in title, an unsafe href as a span, images from imageId alone
// and never from src, and no <a> for a link mark around an atom. Marks
// nest as DOMSerializer nests them: marks shared with the previous node
// stay open. What chips, mentions and page links show comes from a
// Resolver at render time. BlockCache keeps the HTML of unchanged blocks.
// Text gives the search text, References the issue keys, person ids and
// page ids of a document. Diff compares two versions (top-level blocks
// matched by content, then words, atoms and marks of paired blocks with
// Myers' algorithm) and renders <ins class="diff-ins"> and
// <del class="diff-del">. RestoreStep turns the current version into an
// old one with one replace step between FindDiffStart and FindDiffEnd.
// Markdown exports a page.
//
// # The oracle
//
// The differential tests run the real prosemirror-model and
// prosemirror-transform in goja inside go test, without Node: the bundle
// testdata/oracle/oracle.js (committed) holds them, the schema from
// schema.json, the rebase and the fixture generator. goja is imported by
// tests only. Its String.prototype.split differs from V8's for the regular
// expression of prosemirror-model's content-expression tokenizer, so the
// test helper patches that one call when it loads the bundle. Every fixture
// document must parse and write back byte-equal, check must agree on valid
// and hand-made invalid documents with the same message, every batch and
// rebased commit must end with the oracle's steps and V8's document, and
// FuzzRebase has the oracle's generator make documents and concurrent
// commits from a seed that Go must rebase to the same steps.
//
// To change the ProseMirror versions or the generator, and regenerate the
// fixtures in V8:
//
//	cd internal/doc/testdata/oracle
//	npm ci             # versions pinned in package.json and package-lock.json
//	./build.sh         # esbuild: oracle.js
//	node gen-fixtures.mjs
//	cd ../../../.. && go test ./internal/doc/
//	go test ./internal/doc/ -run '^$' -fuzz FuzzRebase -fuzztime 10m
package doc
