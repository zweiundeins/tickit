// The harness's HTTP server: internal/web/static as lith's edge.Static serves
// it (a hashed name finds its file), the harness pages with a CSP like lith's
// DefaultCSP and a nonce, and endpoints for checking what @post returns.
import { createServer } from 'node:http'
import { randomBytes } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { dirname, extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const roots = {
	'/static/': join(here, '../../internal/web/static'),
	'/test/out/': join(here, 'out'),
	'/test/': join(here, 'harness'),
}
const types = { '.js': 'text/javascript; charset=utf-8', '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8', '.json': 'application/json', '.svg': 'image/svg+xml', '.txt': 'text/plain; charset=utf-8' }

// lith's unhash: name.<8 to 64 hex>.ext is name.ext.
const unhash = (p) => p.replace(/\.[0-9a-f]{8,64}(\.[a-z]+)$/, '$1')

// csp=default is lith's DefaultCSP; csp=strict drops 'unsafe-eval' (Datastar
// then compiles with the nonce, from <html data-nonce>).
function csp(nonce, strict) {
	return [
		"default-src 'self'",
		`script-src 'self' 'nonce-${nonce}'${strict ? '' : " 'unsafe-eval'"}`,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"connect-src 'self'",
		"font-src 'self'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
	].join('; ')
}

const image = (id) =>
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 30"><rect width="40" height="30" fill="#9cd"/><text x="4" y="18" font-size="8">${id.replace(/[^\w.-]/g, '')}</text></svg>`

const hits = { drop: 0 }

export function startServer(port = 0) {
	const server = createServer(async (req, res) => {
		const url = new URL(req.url, 'http://x')
		const p = url.pathname
		try {
			if (p.startsWith('/cmd/test/status/')) {
				let body = ''
				for await (const c of req) body += c
				res.writeHead(+p.split('/').pop() || 204).end()
				return
			}
			if (p === '/cmd/test/drop') return void (hits.drop++, req.socket.destroy())
			if (p === '/cmd/test/hits') return void res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify(hits))
			if (p.startsWith('/img/lib/')) return void res.writeHead(200, { 'Content-Type': 'image/svg+xml' }).end(image(p.slice(9)))
			const prefix = Object.keys(roots).find((r) => p.startsWith(r))
			if (!prefix) return void res.writeHead(404).end()
			const rel = normalize(p.slice(prefix.length)).replace(/^(\.\.[/\\])+/, '')
			let file = join(roots[prefix], rel)
			let body
			try {
				body = await readFile(file)
			} catch {
				file = join(roots[prefix], unhash(rel))
				body = await readFile(file)
			}
			const ext = extname(file)
			const headers = { 'Content-Type': types[ext] ?? 'application/octet-stream', 'Cache-Control': 'no-cache' }
			if (ext === '.html') {
				const nonce = randomBytes(12).toString('base64')
				const strict = url.searchParams.get('csp') === 'strict'
				headers['Content-Security-Policy'] = csp(nonce, strict)
				body = body.toString('utf8').replaceAll('{{nonce}}', nonce).replace('{{htmlattrs}}', strict ? ` data-nonce="${nonce}"` : '')
			}
			res.writeHead(200, headers).end(body)
		} catch {
			res.writeHead(404).end()
		}
	})
	return new Promise((resolve) => server.listen(port, '127.0.0.1', () => resolve({ server, url: `http://127.0.0.1:${server.address().port}` })))
}

if (import.meta.url === `file://${process.argv[1]}`) {
	const { url } = await startServer(+(process.argv[2] ?? 8099))
	console.log(`harness on ${url}/test/tab.html?page=p1&tab=t1&name=Ann&hue=3`)
}
