// Commands, input rules (FR-82.2) and keymaps of the editor. The toolbar runs
// the same commands by name.
import { InputRule, inputRules, textblockTypeInputRule, wrappingInputRule, undoInputRule } from 'prosemirror-inputrules'
import { keymap } from 'prosemirror-keymap'
import { baseKeymap, chainCommands, exitCode, lift, setBlockType, toggleMark, wrapIn } from 'prosemirror-commands'
import { liftListItem, sinkListItem, splitListItem, wrapInList } from 'prosemirror-schema-list'
import { redo, undo } from 'prosemirror-history'
import {
	addColumnAfter,
	addColumnBefore,
	addRowAfter,
	addRowBefore,
	deleteColumn,
	deleteRow,
	deleteTable,
	goToNextCell,
	isInTable,
	toggleHeaderRow,
} from 'prosemirror-tables'
import { NodeSelection, TextSelection } from 'prosemirror-state'
import { schema } from './schema.js'

const { nodes: N, marks: M } = schema

// The list node around the selection, if any, with its depth.
function listAround(state) {
	const { $from } = state.selection
	for (let d = $from.depth; d > 0; d--) {
		const t = $from.node(d).type
		if (t === N.bullet_list || t === N.ordered_list || t === N.task_list) return { type: t, depth: d }
	}
	return null
}

const inside = (state, type) => {
	const { $from } = state.selection
	for (let d = $from.depth; d > 0; d--) if ($from.node(d).type === type) return d
	return 0
}

// toggleList wraps in a list of type, or lifts out of it, or changes the
// list around the selection to type.
const toggleList = (type) => (state, dispatch) => {
	const around = listAround(state)
	const item = type === N.task_list ? N.task_item : N.list_item
	if (around?.type === type) return liftListItem(item)(state, dispatch)
	if (around) {
		const $from = state.selection.$from
		const pos = $from.before(around.depth)
		const list = $from.node(around.depth)
		// Bullet and numbered lists swap in place; task lists hold other items.
		if ((around.type === N.task_list) === (type === N.task_list)) {
			if (dispatch) dispatch(state.tr.setNodeMarkup(pos, type, type === N.ordered_list ? { order: 1 } : null))
			return true
		}
		if (!dispatch) return true
		const otherItem = item === N.task_item ? N.list_item : N.task_item
		const tr = state.tr
		list.forEach((child, offset) => {
			if (child.type === otherItem) tr.setNodeMarkup(tr.mapping.map(pos + 1 + offset), item, item === N.task_item ? { checked: false } : null)
		})
		tr.setNodeMarkup(tr.mapping.map(pos), type, type === N.ordered_list ? { order: 1 } : null)
		dispatch(tr)
		return true
	}
	return wrapInList(type)(state, dispatch)
}

const toggleBlock = (type, attrs) => (state, dispatch) => {
	const { $from } = state.selection
	const active = $from.parent.type === type && (!attrs || Object.entries(attrs).every(([k, v]) => $from.parent.attrs[k] === v))
	return active ? setBlockType(N.paragraph)(state, dispatch) : setBlockType(type, attrs)(state, dispatch)
}

const toggleQuote = (state, dispatch) => (inside(state, N.blockquote) ? lift(state, dispatch) : wrapIn(N.blockquote)(state, dispatch))

const insertRule = (state, dispatch) => {
	const { $from } = state.selection
	const index = $from.index(0)
	if (!state.doc.canReplaceWith(index, index, N.horizontal_rule) && !$from.parent.isTextblock) return false
	if (dispatch) {
		const tr = state.tr.replaceSelectionWith(N.horizontal_rule.create())
		dispatch(tr.scrollIntoView())
	}
	return true
}

const insertTable = (state, dispatch) => {
	if (isInTable(state)) return false
	const cell = () => N.table_cell.createAndFill()
	const header = () => N.table_header.createAndFill()
	const row = (make) => N.table_row.create(null, [make(), make(), make()])
	const table = N.table.create(null, [row(header), row(cell), row(cell)])
	if (dispatch) {
		const tr = state.tr.replaceSelectionWith(table)
		const start = tr.selection.from
		// Put the caret into the first cell of the new table.
		let into = null
		tr.doc.nodesBetween(Math.max(0, start - table.nodeSize - 2), Math.min(tr.doc.content.size, start + 2), (n, pos) => {
			if (into == null && n.type === N.table_header) into = pos + 2
		})
		if (into != null) tr.setSelection(TextSelection.near(tr.doc.resolve(into)))
		dispatch(tr.scrollIntoView())
	}
	return true
}

// toggleTask checks or unchecks the task item around the selection.
export const toggleTask = (state, dispatch) => {
	const d = inside(state, N.task_item)
	if (!d) return false
	if (dispatch) {
		const pos = state.selection.$from.before(d)
		dispatch(state.tr.setNodeAttribute(pos, 'checked', !state.doc.nodeAt(pos).attrs.checked))
	}
	return true
}

export const setLanguage = (lang) => (state, dispatch) => {
	const d = inside(state, N.code_block)
	if (!d) return false
	if (dispatch) dispatch(state.tr.setNodeAttribute(state.selection.$from.before(d), 'language', lang))
	return true
}

export const hardBreak = (state, dispatch) => {
	if (state.selection.$from.parent.type.spec.code) return false
	if (dispatch) dispatch(state.tr.replaceSelectionWith(N.hard_break.create()).scrollIntoView())
	return true
}

// The toolbar's commands by name: run, and active for aria-pressed.
export const COMMANDS = {
	h1: { run: toggleBlock(N.heading, { level: 1 }), active: (s) => s.selection.$from.parent.type === N.heading && s.selection.$from.parent.attrs.level === 1 },
	h2: { run: toggleBlock(N.heading, { level: 2 }), active: (s) => s.selection.$from.parent.type === N.heading && s.selection.$from.parent.attrs.level === 2 },
	h3: { run: toggleBlock(N.heading, { level: 3 }), active: (s) => s.selection.$from.parent.type === N.heading && s.selection.$from.parent.attrs.level === 3 },
	strong: { run: toggleMark(M.strong), active: (s) => markActive(s, M.strong) },
	em: { run: toggleMark(M.em), active: (s) => markActive(s, M.em) },
	strike: { run: toggleMark(M.strike), active: (s) => markActive(s, M.strike) },
	code: { run: toggleMark(M.code), active: (s) => markActive(s, M.code) },
	bullet: { run: toggleList(N.bullet_list), active: (s) => listAround(s)?.type === N.bullet_list },
	ordered: { run: toggleList(N.ordered_list), active: (s) => listAround(s)?.type === N.ordered_list },
	task: { run: toggleList(N.task_list), active: (s) => listAround(s)?.type === N.task_list },
	quote: { run: toggleQuote, active: (s) => inside(s, N.blockquote) > 0 },
	codeblock: { run: toggleBlock(N.code_block), active: (s) => s.selection.$from.parent.type === N.code_block },
	rule: { run: insertRule },
	table: { run: insertTable },
	rowBefore: { run: addRowBefore },
	rowAfter: { run: addRowAfter },
	rowDelete: { run: deleteRow },
	colBefore: { run: addColumnBefore },
	colAfter: { run: addColumnAfter },
	colDelete: { run: deleteColumn },
	headerRow: { run: toggleHeaderRow },
	tableDelete: { run: deleteTable },
	undo: { run: undo },
	redo: { run: redo },
}

export function markActive(state, type) {
	const { from, $from, to, empty } = state.selection
	if (empty) return !!type.isInSet(state.storedMarks || $from.marks())
	return state.doc.rangeHasMark(from, to, type)
}

export const inTable = isInTable
export const inCode = (s) => s.selection.$from.parent.type === N.code_block

// "---" becomes a horizontal rule followed by an empty paragraph.
const ruleRule = new InputRule(/^---$/, (state, match, start) => {
	const $start = state.doc.resolve(start)
	if ($start.parent.type !== N.paragraph || $start.parent.textContent.length > 2) return null
	const from = $start.before(), to = $start.after()
	const tr = state.tr.replaceWith(from, to, [N.horizontal_rule.create(), N.paragraph.create()])
	return tr.setSelection(TextSelection.create(tr.doc, from + 2))
})

// An issue key followed by a space becomes a chip (FR-100.1); Backspace right
// after it takes the chip back to text.
const chipRule = new InputRule(/(?:^|[\s(])([A-Z]{2,10}-[1-9][0-9]{0,6}) $/, (state, match, start, end) => {
	if (state.selection.$from.parent.type.spec.code) return null
	const key = match[1]
	// The typed space is not in the document yet: the key ends at end.
	return state.tr.replaceWith(end - key.length, end, [N.issue_chip.create({ key }), schema.text(' ')])
})

export function rules() {
	return inputRules({
		rules: [
			textblockTypeInputRule(/^(#{1,3})\s$/, N.heading, (m) => ({ level: m[1].length })),
			wrappingInputRule(/^\s*([-*])\s$/, N.bullet_list),
			wrappingInputRule(
				/^(\d+)\.\s$/,
				N.ordered_list,
				(m) => ({ order: Math.min(9999, Math.max(1, +m[1])) }),
				(m, node) => node.childCount + node.attrs.order === +m[1],
			),
			wrappingInputRule(/^\s*\[([ xX]?)\]\s$/, N.task_list),
			wrappingInputRule(/^\s*>\s$/, N.blockquote),
			textblockTypeInputRule(/^```$/, N.code_block),
			ruleRule,
			chipRule,
		],
	})
}

const mac = typeof navigator !== 'undefined' && /Mac|iP(hone|[oa]d)/.test(navigator.platform)

export function keymaps({ openLink, focusToolbar }) {
	const keys = {
		'Mod-z': undo,
		'Shift-Mod-z': redo,
		'Mod-y': redo,
		Backspace: undoInputRule,
		'Mod-b': toggleMark(M.strong),
		'Mod-B': toggleMark(M.strong),
		'Mod-i': toggleMark(M.em),
		'Mod-I': toggleMark(M.em),
		'Mod-Shift-s': toggleMark(M.strike),
		'Mod-Shift-S': toggleMark(M.strike),
		'Mod-e': toggleMark(M.code),
		'Mod-k': openLink,
		'Mod-K': openLink,
		'Alt-F10': focusToolbar,
		Enter: chainCommands(splitListItem(N.list_item), splitListItem(N.task_item, { checked: false })),
		'Shift-Enter': hardBreak,
		'Mod-Enter': chainCommands(exitCode, toggleTask),
		Tab: chainCommands(goToNextCell(1), sinkListItem(N.list_item), sinkListItem(N.task_item)),
		'Shift-Tab': chainCommands(goToNextCell(-1), liftListItem(N.list_item), liftListItem(N.task_item)),
		'Mod-[': chainCommands(liftListItem(N.list_item), liftListItem(N.task_item)),
		'Mod-]': chainCommands(sinkListItem(N.list_item), sinkListItem(N.task_item)),
	}
	if (!mac) keys['Ctrl-Shift-z'] = redo
	return [keymap(keys), keymap(baseKeymap)]
}

export const shortcutText = (keys) => keys.replace(/Mod/g, mac ? 'Cmd' : 'Ctrl')
