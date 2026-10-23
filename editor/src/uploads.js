// Image uploads (FR-85): a file pasted, dropped or chosen goes to the host's
// upload URL as multipart with the tab and an op id; while it travels a
// placeholder widget (not part of the document) holds its place and shows
// the progress. The server answers {id, width, height}, and the image node
// goes where the placeholder went; a refusal comes back in words.
import { Plugin, PluginKey } from 'prosemirror-state'
import { Decoration, DecorationSet } from 'prosemirror-view'

export const MAX_BYTES = 5 << 20
export const TYPES = ['image/png', 'image/jpeg', 'image/webp']

const key = new PluginKey('sb-uploads')

export function uploadPlugin() {
	return new Plugin({
		key,
		state: {
			init: () => DecorationSet.empty,
			apply(tr, set) {
				set = set.map(tr.mapping, tr.doc)
				const a = tr.getMeta(key)
				if (a?.add) set = set.add(tr.doc, [Decoration.widget(a.add.pos, a.add.dom, { id: a.add.id, side: -1, ignoreSelection: true })])
				if (a?.remove) set = set.remove(set.find(undefined, undefined, (s) => s.id === a.remove))
				return set
			},
		},
		props: { decorations: (state) => key.getState(state) },
	})
}

// placeholder adds a placeholder at pos, and reports its element.
export function placeholder(view, id, pos) {
	const dom = document.createElement('span')
	dom.className = 'sb-upload'
	dom.contentEditable = 'false'
	dom.setAttribute('role', 'status')
	dom.textContent = 'Uploading image…'
	view.dispatch(view.state.tr.setMeta(key, { add: { id, pos, dom } }))
	return dom
}

// placed is where a placeholder is now, or null once its text went.
export function placed(state, id) {
	const found = key.getState(state).find(undefined, undefined, (s) => s.id === id)
	return found.length ? found[0].from : null
}

export const removePlaceholder = (tr, id) => tr.setMeta(key, { remove: id })

// imageFiles are the image files of a paste or a drop.
export function imageFiles(data) {
	return [...(data?.files ?? [])].filter((f) => f.type.startsWith('image/') || f.type === '')
}

// check refuses before sending what the server would refuse anyway.
export function check(file) {
	if (file.type && !TYPES.includes(file.type)) return 'Only PNG, JPEG and WebP images can be uploaded.'
	if (file.size > MAX_BYTES) return 'An image has at most 5 MB.'
	if (file.size === 0) return 'This file is empty.'
	return ''
}

const opID = () => 'up' + Math.random().toString(36).slice(2, 12) + Date.now().toString(36)

// send posts file to url as the tab and calls progress(fraction), then
// resolves {id, width, height} or rejects with a message for the person.
export function send(url, tab, file, progress) {
	return new Promise((resolve, reject) => {
		const op = opID()
		const form = new FormData()
		form.append('tab', tab)
		form.append('op', op)
		form.append('file', file, 'image')
		const xhr = new XMLHttpRequest()
		xhr.open('POST', url)
		xhr.setRequestHeader('Datastar-Request', 'true')
		// The server checks the right and the tab's stream before the body.
		xhr.setRequestHeader('Tab-Id', tab)
		xhr.setRequestHeader('Op-Id', op)
		xhr.upload.onprogress = (e) => e.lengthComputable && progress(e.loaded / e.total)
		xhr.onload = () => {
			if (xhr.status === 200) {
				try {
					const r = JSON.parse(xhr.responseText)
					if (r && typeof r.id === 'string') return resolve(r)
				} catch {}
				return reject(new Error('The server answered something else than an image.'))
			}
			const text = xhr.responseText.trim()
			reject(new Error(xhr.status === 400 || !text || text.length > 300 ? 'This upload could not be read.' : text))
		}
		xhr.onerror = () => reject(new Error('The upload did not reach the server. Try again.'))
		xhr.send(form)
	})
}
