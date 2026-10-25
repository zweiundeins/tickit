// Where coedit's shared editor modules are: the editor/ folder of the
// github.com/zweiundeins/coedit module that go.mod names (a local checkout
// while go.mod replaces it). The builds alias 'coedit' to it and resolve its
// ProseMirror imports from this folder's node_modules, also where the
// checkout has a node_modules of its own, so that a bundle holds one copy of
// each package.
import { execFileSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const dir = execFileSync('go', ['list', '-m', '-f', '{{.Dir}}', 'github.com/zweiundeins/coedit'], { cwd: here, encoding: 'utf8' }).trim()

export const coedit = join(dir, 'editor')
const pm = ['prosemirror-model', 'prosemirror-state', 'prosemirror-transform', 'prosemirror-view']
export const shared = {
	alias: { coedit, ...Object.fromEntries(pm.map((p) => [p, join(here, 'node_modules', p)])) },
	nodePaths: [join(here, 'node_modules')],
}
