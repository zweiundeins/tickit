// Package seed holds the demo's seeded content (DM-02) as data: one
// fictional company, Alpenglow, a small Swiss shop for outdoor and hiking
// gear, with six people, three projects of issues, two spaces of pages, five
// page templates and a library of images. Load reads it from the files
// embedded below data/; Check checks it; Parse turns its markup into
// documents of internal/doc/schema.json. The commands that write it into
// the database live elsewhere.
//
// # Files
//
//	data/company.json        people, projects (components, labels), spaces
//	data/images.json         the image library, written by genimages
//	data/issues-<KEY>.json   a project's issues in number order, and "rank",
//	                         the board order from top to bottom
//	data/pages-<KEY>/*.md    a space's pages, one file each
//	data/pages-<KEY>/comments.json  page comments with their replies
//	data/templates/*.md      page templates
//
// Times are never absolute (DM-02.4). A moment is written "-12d 09:40": 12
// days before the run's date at 09:40 in the run's time zone (When). Every
// moment lies in the 30 days before the run's date. A due date is a number
// of days after the run's date; negative is past.
//
// An issue holds its final state. Its "history" lists changes in time order
// (field, from, to, by, at); the state it was created in is, per field, the
// From of its first change (Issue.Initial). Created times grow with the
// issue number. In JSON, a markup text is a string or an array of lines.
//
// A page file starts with front matter: title, parent (the parent's title;
// none for the home page and top-level pages), home (true for the space's
// home page), position among its siblings, author, created, labels. A page
// with several versions lists the earlier ones under "versions" (at, by,
// message, body), oldest first, and says who saved the current body with
// edited, editor and message. A template's front matter holds name and
// description.
//
// # Markup
//
// The markup is a strict subset of Markdown with a few additions for
// tickit's atoms. What it does not define is an error, never literal text.
//
// Blocks, separated by blank lines:
//
//	# Heading            levels 1 to 3 (#, ##, ###)
//	plain lines          a paragraph; lines join with a space, and a line
//	                     ending in \ ends with a hard break
//	> quoted             a quote holding blocks
//	- item               a bullet list; 1. starts a numbered list
//	- [ ] open           a task list; - [x] is a done task
//	```go                fenced code, with a language of the allowlist
//	                     (go, js, ts, html, css, sql, json, yaml, bash,
//	                     python, java, rust, markdown, diff, text) or none
//	| A | B |            a table: a header row, a separator row
//	| --- | --- |        (| --- | --- |), then rows of as many cells;
//	                     \| is a pipe inside a cell; at most 50 rows by 20
//	---                  a horizontal rule
//	![Alt text](lib:id)  an image of the library, on a line of its own
//
// A list item's further lines, nested lists and code are indented by two
// spaces (by the marker's width for numbered items).
//
// Inline:
//
//	**strong**  *em* or _em_  ~~strike~~  `code`
//	[text](https://example.com)   links: http, https and mailto only
//	[[WEB-12]]                    an issue chip
//	[[page:Exact Title]]          a link to a page of the same space
//	[[page:ENG/Exact Title]]      a link to a page of another space
//	@s-priya                      a mention of a seeded person
//	\*                            a backslash makes punctuation literal
//
// Issue descriptions and comments link pages with the space's key. Comments
// hold no headings and no tables.
package seed

//go:generate go run ./genimages -out ../../web/static/img/lib -index data/images.json
