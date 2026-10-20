// Builds sb-rich-editor into internal/web/static/editor/ (committed, so the Go
// build needs no Node):
//
//   sb-rich-editor.min.js  the Rocket component, small; imports 'datastar'
//   pm-core.min.js         ProseMirror and the editor, loaded on first use
//   LICENSES.txt           the licences of what the bundles hold
//
// lith's edge.Static serves a file as /static/<name>.<sha256[:12]>.<ext> with
// an immutable cache, and *.min.js as written. The component imports the core
// by exactly that name, computed here, so the import resolves to the
// immutable URL; under any other hash length lith still finds the file by
// its plain name (with no-cache).
//
// node build.mjs [--watch-free]   prints the sizes (minified, gzip, brotli).
import { build } from 'esbuild'
import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { brotliCompressSync, gzipSync, constants } from 'node:zlib'

const here = dirname(fileURLToPath(import.meta.url))
const out = join(here, '../internal/web/static/editor')
mkdirSync(out, { recursive: true })

const common = { bundle: true, format: 'esm', target: 'es2022', minify: true, legalComments: 'none', charset: 'utf8', logLevel: 'warning' }

// 1. The core.
const coreFile = 'pm-core.min.js'
await build({ ...common, entryPoints: [join(here, 'src/core.js')], outfile: join(out, coreFile) })
const core = readFileSync(join(out, coreFile))
const hash = createHash('sha256').update(core).digest('hex').slice(0, 12)
const coreURL = `./pm-core.min.${hash}.js`

// 2. The component, importing the core by its hashed name.
const entry = readFileSync(join(here, 'src/sb-rich-editor.js'), 'utf8')
if (!entry.includes("const CORE = './pm-core.js'")) throw new Error('build: CORE constant not found in src/sb-rich-editor.js')
await build({
	...common,
	stdin: { contents: entry.replace("const CORE = './pm-core.js'", `const CORE = '${coreURL}'`), resolveDir: join(here, 'src'), sourcefile: 'sb-rich-editor.js' },
	external: ['datastar', './pm-core.*'],
	outfile: join(out, 'sb-rich-editor.min.js'),
})

// 3. Licences.
const lic = (pkg) => readFileSync(join(here, 'node_modules', pkg, 'LICENSE'), 'utf8').trim()
const pkgs = ['prosemirror-model', 'prosemirror-state', 'prosemirror-view', 'prosemirror-transform', 'prosemirror-keymap', 'prosemirror-history', 'prosemirror-inputrules', 'prosemirror-commands', 'prosemirror-schema-list', 'prosemirror-gapcursor', 'prosemirror-dropcursor', 'prosemirror-tables', 'orderedmap', 'rope-sequence', 'w3c-keyname']
const versions = pkgs.map((p) => `${p}@${JSON.parse(readFileSync(join(here, 'node_modules', p, 'package.json'), 'utf8')).version}`)
writeFileSync(
	join(out, 'LICENSES.txt'),
	[
		'sb-rich-editor bundles the following packages. Their licences follow.',
		'',
		...versions,
		'@stepwisehq/prosemirror-collab-commit@1.0.5 (adapted in editor/src/collab.js)',
		'',
		...pkgs.map((p) => `--- ${p}\n\n${lic(p)}\n`),
		`--- @stepwisehq/prosemirror-collab-commit\n\n${readFileSync(join(here, 'node_modules/@stepwisehq/prosemirror-collab-commit/LICENSE'), 'utf8').trim()}\n`,
	].join('\n'),
)

// 4. Sizes.
const size = (f) => {
	const b = readFileSync(join(out, f))
	const kb = (n) => (n / 1024).toFixed(1) + ' KB'
	return `${f.padEnd(24)} ${kb(b.length).padStart(9)} min ${kb(gzipSync(b, { level: 9 }).length).padStart(9)} gzip ${kb(brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }).length).padStart(9)} brotli`
}
console.log(size('sb-rich-editor.min.js'))
console.log(size(coreFile))
console.log(`core imported as ${coreURL}`)
