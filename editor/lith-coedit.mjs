// Where lith-coedit's shared editor modules are: the editor/ folder of the
// github.com/zweiundeins/lith-coedit module that go.mod names (a local checkout
// while go.mod replaces it). The builds alias 'lith-coedit' to it and resolve its
// ProseMirror imports from this folder's node_modules, also where the
// checkout has a node_modules of its own, so that a bundle holds one copy of
// each package.
import { execFileSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const dir = execFileSync('go', ['list', '-m', '-f', '{{.Dir}}', 'github.com/zweiundeins/lith-coedit'], { cwd: here, encoding: 'utf8' }).trim()
if (!dir) throw new Error('lith-coedit.mjs: go list names no directory for github.com/zweiundeins/lith-coedit; run go mod download github.com/zweiundeins/lith-coedit')

export const coedit = join(dir, 'editor')
const pm = ['prosemirror-model', 'prosemirror-state', 'prosemirror-transform', 'prosemirror-view']
export const shared = {
	alias: { 'lith-coedit': coedit, ...Object.fromEntries(pm.map((p) => [p, join(here, 'node_modules', p)])) },
	nodePaths: [join(here, 'node_modules')],
}
