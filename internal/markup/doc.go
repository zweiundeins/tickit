// Package markup reads and writes tickit's text markup: a subset of
// Markdown with tickit's atoms, which the seed's content is written in
// (internal/demo/seed documents it) and which people type into
// descriptions and comments until the rich editor edits them (Q22).
//
// Parse reads it strictly, for the seed: what the markup does not define
// is an error. ParseMode with Text or Comment reads what people type: what
// the markup does not define stays text, and only the document's limits
// refuse it. Write turns a document back into markup, so an existing
// description can be edited as text.
//
// Inline atoms: [[WEB-12]] is an issue chip, [[page:Title]] a page link,
// @<person id> a mention (s-priya, g-…, a-…).
package markup
