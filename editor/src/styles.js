// Structural CSS for the light-DOM editor, adopted into the document once:
// what ProseMirror, prosemirror-tables and the gap cursor need to edit
// correctly, and fallbacks for carets, chips, tasks and images. The design
// system (static/css/prose.css, editor.css) does the looks: every rule here
// is in @layer base with :where() selectors, so any of its rules wins.
export const documentCSS = /* css */ `
@layer reset, tokens, theme, base, layout, components, utilities;
@layer base {
:where(.ProseMirror) { position: relative; word-wrap: break-word; white-space: pre-wrap; white-space: break-spaces; font-variant-ligatures: none; font-feature-settings: "liga" 0; outline: none; }
:where(.ProseMirror) :where(pre) { white-space: pre-wrap; }
:where(.ProseMirror) :where(li) { position: relative; }
:where(.ProseMirror-hideselection) *::selection { background: transparent; }
:where(.ProseMirror-hideselection) { caret-color: transparent; }
:where(.ProseMirror) :where([draggable][contenteditable=false]) { user-select: text; }
:where(.ProseMirror-selectednode) { outline: 2px solid var(--tk-focus, Highlight); outline-offset: 1px; }
:where(li.ProseMirror-selectednode) { outline: none; }
:where(img.ProseMirror-separator) { display: inline !important; border: none !important; margin: 0 !important; }
:where(.ProseMirror-gapcursor) { display: none; pointer-events: none; position: absolute; }
:where(.ProseMirror-gapcursor)::after { content: ""; display: block; position: absolute; top: -2px; width: 20px; border-top: 1px solid currentColor; animation: sb-gapcursor-blink 1.1s steps(2, start) infinite; }
@keyframes sb-gapcursor-blink { to { visibility: hidden; } }
:where(.ProseMirror-focused) :where(.ProseMirror-gapcursor) { display: block; }
:where(.ProseMirror) :where(table) { border-collapse: collapse; table-layout: fixed; width: 100%; overflow: hidden; }
:where(.ProseMirror) :where(td, th) { vertical-align: top; box-sizing: border-box; position: relative; min-width: 3em; }
:where(.ProseMirror) :where(.selectedCell)::after { z-index: 2; position: absolute; content: ""; inset: 0; background: var(--tk-selection, rgb(120 160 255 / .3)); pointer-events: none; }
:where(.ProseMirror) :where(li.task-item) { list-style: none; display: flex; gap: .5em; align-items: baseline; }
:where(.ProseMirror) :where(li.task-item) > :where(div) { flex: 1; min-width: 0; }
:where(.ProseMirror) :where(li.task-item) > :where(input) { margin: 0; cursor: pointer; }
:where(.ProseMirror) :where(figure.doc-image) { position: relative; margin-inline: 0; }
:where(.ProseMirror) :where(figure.doc-image img) { display: block; max-width: 100%; height: auto; }
:where(.ProseMirror) :where(.doc-image__alt) { position: absolute; inset-block-start: .5em; inset-inline-end: .5em; opacity: 0; }
:where(.ProseMirror) :where(figure.doc-image:hover, figure.doc-image.ProseMirror-selectednode, figure.doc-image:focus-within) :where(.doc-image__alt) { opacity: 1; }
:where(.ProseMirror) :where(.issue-chip, .mention, .page-link) { cursor: default; }
:where(.tk-caret) { position: relative; margin-inline: -1px; border-inline-start: 2px solid currentColor; word-break: normal; pointer-events: auto; }
:where(.tk-caret__tag) { position: absolute; inset-block-end: 100%; inset-inline-start: -2px; white-space: nowrap; user-select: none; pointer-events: none; font-size: .6875rem; line-height: 15px; padding: 0 4px; opacity: 0; transition: opacity .2s; }
:where(.tk-caret--tag, .tk-caret:hover) :where(.tk-caret__tag) { opacity: 1; }
:where(.tk-caret--idle) :where(.tk-caret__tag) { opacity: 0; }
:where(.tk-remote-sel) { background: color-mix(in srgb, currentColor 18%, transparent); }
@media (prefers-reduced-motion: reduce) { :where(.tk-caret__tag) { transition: none; } }
@media (forced-colors: active) { :where(.tk-caret) { border-color: Highlight; } :where(.tk-caret__tag) { background: Highlight; color: HighlightText; } :where(.tk-remote-sel) { background: Highlight; color: HighlightText; } }
}
`
