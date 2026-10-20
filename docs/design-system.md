# tickit's design system

Every screen of tickit is built from `internal/ui` (templ components with plain Go props) and the stylesheets in `internal/web/static/css/`. `/design` renders every component in every state, with composed screens to copy; `/design?theme=light&style=smooth&density=compact` shows the other looks. This page says how the parts fit and how to use them.

The look: tide (a teal-cyan, hue 203) as the one signal colour, slate neutrals so that status, type and priority carry the colour on dense screens, the system face for reading, JetBrains Mono for labels, keys and counts, Pixelify Sans for display text only, and 8-bit corners that the smooth style rounds. It is a sibling of lith's site and Starbase: the same layers, the same token mechanics, the same pending pixel.

## Principles

- **Tokens, never raw values.** Components and pages read `--tk-*` tokens. No hex, no `oklch()` and no pixel sizes outside `tokens.css`, except hairlines (`1px`) and the pixel art itself.
- **No `style=` in markup.** Server HTML carries classes and data attributes. A runtime value goes through a data attribute with a rule per value (`data-hue="7"`) or through `data-style:*` from Datastar, which writes CSSOM. `TestDesign` fails on a `style` attribute.
- **Colour is never alone** (NFR-50). Types, priorities and status categories each have their own shape and their name; overdue has a warning icon and the word; a refused card shakes and says why; a diff underlines and strikes.
- **Every icon has a name.** An icon sits beside its text (`ui.Icon`, decorative), or names itself (`ui.LabeledIcon`, `ui.TypeIcon`, `ui.PriorityIcon`) and shows the name in a tooltip.
- **Focus is always visible.** A 2px ring in `--tk-focus`. Menus, the page tree, the editor toolbar and the board's card menu all work by keyboard (NFR-47).
- **Pending, then the outcome** (MP-09). A control that sent a change is read-only and `aria-busy` until the frame with its op id; a refusal shows next to the control in a polite live region; a request with no outcome says "This did not go through." with Retry.
- **Data stays in the UI face.** Lane names, issue summaries, page titles and documents hold umlauts and long words; they are never set in Pixelify Sans.
- **No feature behaviour in `internal/ui`.** Components take `templ.Attributes` (`Attrs`, `RetryAttrs`, `CreateAttrs`, `ErrorAttrs`) for the Datastar wiring a feature adds. The expressions they carry themselves (in `internal/ui/expr.go`) are view behaviour: arrow keys, `aria-expanded`, Escape for tooltips. All are constants, none holds a person's value (NFR-13).

## Files and how a page loads them

| File | Layer | Holds |
| --- | --- | --- |
| `css/tokens.css` | tokens, theme | the layer order, `@font-face`, primitives, semantic tokens, the hue tokens |
| `css/starbase.css` | theme, components | `--sb-*` mapped onto `--tk-*`; Starbase components before they are defined |
| `css/base.css` | reset, base | reset, type, links, focus, selection, code, reduced motion |
| `css/layout.css` | layout | the app frame, main's widths, `.stack`, `.cluster`, `.split`, page grids |
| `css/components.css` | components | every component of `internal/ui` |
| `css/shell.css` | components | header, demo notice, sidebar, narrow-screen menu, footer |
| `css/prose.css` | components | rich text, inline issue chips, mentions, page links, diffs |
| `css/editor.css` | components | editor frame, toolbar, save status, remote carets and selections |
| `css/board.css` | components | board, lanes, cards and the states of a move |
| `css/app.css` | components | the app's own pages: home, login, privacy, 404 and 410, the person's menu, the first-visit hint, the floating notices |
| `css/utilities.css` | utilities | `.visually-hidden`, `.pixel-frame`, `.muted`, `.mono`, `.truncate` |
| `css/design.css` | components | the styleguide's own layout, loaded by `/design` only |

A page links one stylesheet. `web.BundleCSS` joins `web.Stylesheets` in the order above into `css/tickit.css`, with the font URLs rewritten to their hashed names, and `edge.Static` fingerprints, minifies and precompresses it. One request, nothing imported at runtime, and the font preloads ask for the same immutable URLs as the stylesheet. A new stylesheet goes into `web.Stylesheets`. `web.New` registers the bundle and `art/mark.svg` (the favicon, `web.Mark`), and every page links them and preloads the fonts (`views.Layout`).

The head of a page, as `/design` writes it:

```html
<html lang="en" data-tk-theme="dark" data-tk-style="pixel" data-tk-density="comfortable">
<meta name="color-scheme" content="dark">
<link rel="icon" type="image/svg+xml" href="/static/art/mark.<hash>.svg">
<link rel="preload" href="/static/fonts/jetbrains-mono.<hash>.woff2" as="font" type="font/woff2" crossorigin>
<link rel="preload" href="/static/fonts/pixelify-sans.<hash>.woff2" as="font" type="font/woff2" crossorigin>
<link rel="stylesheet" href="/static/css/tickit.<hash>.css">
<script type="importmap" nonce="…">{"imports":{"datastar":"/static/vendor/datastar-rocket.<hash>.js"}}</script>
<script type="module" src="/static/vendor/datastar-rocket.<hash>.js"></script>
<script type="module" src="/static/vendor/starbase/select/select.<hash>.js"></script>
```

`ui.Prefs{Theme, Style, Density}` gives the `<html>` attributes (`Prefs.Attrs()`) and the `color-scheme` meta (`Prefs.ColorScheme()`). The app's pages take the session's choice from its prefs row (`tabs.Prefs`; NFR-31 allows one cookie only), with `system` as the default theme; `/design` takes it from the query.

## Themes, styles and density

A theme is a `color-scheme`: every semantic token is `light-dark(light, dark)`. `data-tk-theme="dark|light|system"` works on `<html>` and on any element, so a panel can show the other theme. `data-tk-style="smooth"` sets `--tk-notch: 0` (polygons become boxes and radii take over), a 1px frame step and the UI face for display text. `data-tk-density="compact"` makes rows 1.75rem and medium controls 1.75rem.

## Tokens

The semantic tokens components read. Primitives (`--tk-slate-0` to `13`, `--tk-tide-1` to `10`) are for `tokens.css` only.

| Token | Use |
| --- | --- |
| `--tk-bg` | the page |
| `--tk-surface-1` | panels, cards, tables |
| `--tk-surface-2` | table heads, chips, counts, footers of layers |
| `--tk-surface-inset` | fields, lanes, code |
| `--tk-surface-hover`, `--tk-surface-selected` | hovered rows, selected rows |
| `--tk-surface-overlay`, `--tk-scrim` | menus and dialogs, the backdrop behind them |
| `--tk-border`, `--tk-border-subtle`, `--tk-border-strong` | edges, lines between regions, hovered edges |
| `--tk-control-border` | a field's edge (3:1 against its surroundings) |
| `--tk-text-1`, `--tk-text-2`, `--tk-text-muted` | text, secondary text, meta and hints |
| `--tk-accent`, `--tk-on-accent`, `--tk-accent-strong` | the primary fill, text on it, its hover |
| `--tk-accent-text`, `--tk-accent-edge`, `--tk-accent-subtle` | links and current items, the edge of accent fills, washes |
| `--tk-focus`, `--tk-glow`, `--tk-selection` | focus rings, glows, text selection |
| `--tk-ok`, `--tk-warn`, `--tk-danger`, `--tk-info` | status as text, each with `-subtle` for a wash |
| `--tk-on-danger` | text on a danger fill |
| `--tk-tip-bg`, `--tk-tip-text` | the inverted tooltip and name tag bubble |
| `--tk-code-*` | code blocks: bg, text, keyword, function, string, number, comment |
| `--tk-shadow-1`, `-2`, `-overlay` | small, card, overlay (one shadow without spread) |

**Hues.** An element that carries a hue sets `--h` through an attribute, and reads `--tk-hue-text`, `--tk-hue-fill`, `--tk-hue-on` (a glyph on the fill), `--tk-hue-wash`, `--tk-hue-edge` and `--tk-hue-tile`. The attributes: `data-type` (epic 295, story 145, task 235, bug 25, subtask 200), `data-prio` (low 235, medium 80, high 50, critical 22), `data-cat` (todo is neutral, progress 250, done 150) and `data-hue="0"` to `"15"` (faces, carets, selections, epics and labels, 22.5 degrees apart). The tokens are declared on those elements, since a custom property that refers to `--h` is resolved where it is declared.

**Space, sizes, motion.** Space is a 4px rhythm: `--tk-space-0_5` (2px) to `--tk-space-16` (4rem). Controls are `--tk-control-sm|md|lg` (28, 32, 40px), rows `--tk-row-height` (2rem). The header is `--tk-header-height` (3rem), the sidebar `--tk-sidebar-width` (15rem), documents `--tk-prose-max` (46rem), pages `--tk-content-max` (80rem), the side panel `--tk-panel-width` (40rem), lanes `--tk-lane-width` (17rem, at least `--tk-lane-min`, 260px, NFR-56). Motion: `--tk-dur-1` to `5` (120 to 640ms); `--tk-ease-glide` puts things into place, `--tk-ease-snap` answers a press, `--tk-ease-zoom` opens a layer, `--tk-ease-steps` is pixel motion. Reduced motion turns everything off. Stacking: `--tk-z-raised`, `-caret`, `-sticky`, `-dropdown`, `-overlay`, `-modal`, `-toast`, `-tooltip`.

`TestContrast` (`internal/web/contrast_test.go`) reads `tokens.css` and checks every text token on every surface at 4.5:1, focus rings and field edges at 3:1, text on washes composited over each surface, and each hue's text, fill, glyph and tile, in both themes. Change a colour and run it.

## Type scale

| Token | Size | Use |
| --- | --- | --- |
| `--tk-text-2xs` | 11px | lozenges, eyebrows (`.tk-eyebrow`), counts, keys on cards |
| `--tk-text-xs` | 12px | meta, timestamps, buttons, table heads |
| `--tk-text-sm` | 13px | table cells, cards, menus |
| `--tk-text-md` | 14px | UI body, forms, comments |
| `--tk-text-lg` | 16px | documents and descriptions |
| `--tk-text-xl` | 18px | document h3, dialog titles |
| `--tk-text-2xl` | 22px | issue summary, document h2 |
| `--tk-text-3xl` | 28px | page titles |
| `--tk-text-4xl` | 36px | display text |

`--tk-font-ui` is the system face, `--tk-font-mono` JetBrains Mono (buttons, nav links, keys, lozenges, counts, code), `--tk-font-display` Pixelify Sans (the brand, styleguide headings, empty states). Both font files are Starbase's subsets with Latin-1 letters, self-hosted with their OFL texts in `static/fonts/` (NFR-29).

## Components

Each takes a props struct; zero values are the defaults. The calls below are templ.

### Frame

```templ
@ui.Shell(ui.ShellProps{
	Header:  ui.HeaderProps{Nav: ui.MainNav("projects"), CreateFor: "create", Unread: 3,
		Notifications: notifications(), Me: me, MeMenu: meMenu(), Display: ui.DisplayProps{Prefs: prefs, Attrs: prefAttrs}},
	Sidebar: ui.SideNav(ui.SideNavProps{Title: "Website", Key: "WEB", Href: "/WEB", Sections: sections, Collapsible: true}),
	Notice:  ui.DemoNotice(ui.DemoProps{NextReset: "5 h", PrivacyHref: "/privacy"}),
	Footer:  ui.FooterProps{Links: []ui.NavItem{{Label: "Privacy", Href: "/privacy"}}},
	Width:   "full", // "", "prose" or "full"
}) {
	…the page…
}
```

The header holds the brand, Projects, Spaces, Filters and Dashboards with the current one marked, `Admin` links for those who may use them, search with its Ctrl K hint (the shortcut works on every page), Create, the bell (a button with `aria-expanded` and a polite live region `#tk-unread` for the count), the display menu and the person's menu (FR-137). Below 48rem the links and the sidebar move into a panel that the menu button opens. The shell hides tooltips on Escape. `DisplayProps` takes `Href` (links) or `Attrs` (buttons that post a preference).

### Buttons and fields

```templ
@ui.Button(ui.ButtonProps{Label: "Create", Icon: "plus", Variant: ui.Primary})
@ui.Button(ui.ButtonProps{Label: "Copy link", Icon: "link", IconOnly: true, Variant: ui.Ghost, Size: ui.SizeSM})
@ui.Button(ui.ButtonProps{Label: "Save", Variant: ui.Primary, Attrs: templ.Attributes{"data-attr:aria-busy": pendingExpr}})

{{ f := ui.FieldProps{ID: "summary", Label: "Summary", Hint: "One line.", Required: true, ErrorAttrs: refusalAttrs} }}
@ui.Field(f) {
	@ui.Input(ui.InputProps{ID: f.ID, Name: "summary", DescribedBy: f.DescribedBy(), Attrs: inputAttrs})
}
```

Variants are `Primary` (one per view), `Secondary`, `Ghost` and `Danger`. `Command: "show-modal", CommandFor: id` opens a dialog without script. A field's message element is always present and polite, so a refusal that arrives later is read (NFR-48.2); a pending input is `Readonly` (dashed). Also: `ui.Textarea`, `ui.Select` (native, for short plain lists), `ui.Checkbox`, `ui.Toggle` (role=switch), `ui.RadioGroup` (with `Segmented` for a row of choices with icons, using `ui.TypeMark` and `ui.PriorityMark` as option icons), `ui.Picker` (sb-select: searchable, several, remote) and `ui.DatePicker` (sb-date-picker). Pickers carry their own label inside the component, so they take no Field.

### Menus and layers

```templ
@ui.Menu(ui.MenuProps{ID: "sort", Label: "Sort by", Icon: "sort", Caret: true, Variant: ui.Secondary}) {
	@ui.MenuItem(ui.MenuItemProps{Label: "Rank", Checked: ui.Pressed(true), Autofocus: true})
	@ui.MenuSeparator()
	@ui.MenuItem(ui.MenuItemProps{Label: "Delete", Icon: "trash", Danger: true, Attrs: deleteAttrs})
}
@ui.ActionMenu(ui.ActionMenuProps{Label: "Actions", Name: "issue", Items: []ui.MenuEntry{{Value: "watch", Label: "Watch"}, {Label: "Move to", Children: lanes}}, Attrs: selectAttrs})
@ui.Dialog(ui.DialogProps{ID: "create", Title: "Create an issue", Size: ui.SizeLG, Footer: footer()}) { …fields… }
@ui.SidePanel(ui.PanelProps{ID: "issue", Title: summary, Lead: lead()}) { …the issue… }
@ui.Tip("Calm Lynx, away") { …a focusable anchor… }
```

`Menu` is a native popover anchored to its button: Escape and a click outside close it, the focus returns to the button, arrow keys move through items. `ActionMenu` is Starbase's sb-dropdown, for submenus; it emits `sb-select {name, value}`. `Dialog` and `SidePanel` are modal `<dialog>`s: focus stays inside, Escape closes, the focus returns (NFR-47.2). A panel's body is a container of its own, so an issue laid out for a page stacks inside it. Give the first field `autofocus`.

### Navigation

```templ
@ui.Tabs(ui.TabsProps{Labels: []string{"Comments", "History"}}) {
	@ui.TabPanel("Comments") { … }
	@ui.TabPanel("History") { … }
}
@ui.TabNav("Project settings", items)
@ui.Breadcrumbs([]ui.Crumb{{Label: "Engineering", Href: "/spaces/eng", Icon: "space"}, {Label: "Release checklist"}})
@ui.Pagination(ui.PaginationProps{Page: 2, Pages: 7, Total: 312, Noun: "issues", Href: pageURL})
@ui.Tree(ui.TreeProps{Label: "Pages in Engineering", Items: items})
```

`Tree` is the ARIA tree pattern rendered by the server: links as tree items, one tab stop, Up and Down through visible items, Right and Left to open and close, Home and End, Enter to follow. Expand the ancestors of the current page in `Items`. Open and closed state survives frames (`data-preserve-attr`).

### Tables

```templ
@ui.Table(ui.TableProps{Caption: "Open issues", HideCaption: true, Sticky: true}) {
	<thead><tr>@ui.SortHeader(ui.SortProps{Label: "Priority", Dir: "descending", Attrs: sortAttrs})…</tr></thead>
	<tbody><tr aria-selected="true">…</tr></tbody>
}
```

Plain tables (NFR-50). In cells: `.tk-key` for keys, `.tk-grow` for the column that takes the room and cuts its text, `.tk-num` for numbers, `.tk-cell` for a face beside a name. The table scrolls sideways inside its region on narrow screens.

### People

```templ
@ui.Avatar(ui.AvatarProps{Person: ui.Person{Seed: id, Name: "Sunny Otter"}, Size: ui.SizeMD})
@ui.PresenceRow(ui.PresenceProps{Label: "People on this board", People: entries})
@ui.Byline(ui.BylineProps{Person: p, Verb: "edited", At: t, Now: now})
@ui.Mention(id, "Sunny Otter", "/people/"+id)
```

`ui.FaceOf(seed)` draws lith's identicon from the person's id: a mirrored 5 by 5 face and one of sixteen hues, which their caret and selection share. `Person.Hue` sets a person's own hue in its place (a seeded person's); app code makes a `ui.Person` with `views.UIPerson`, which always passes the stored hue, an index of 0 to 15 like `data-hue`. Sizes: `SizeXS` 1rem, `SizeSM` 1.5rem, `SizeMD` 2rem, `SizeLG` 3rem. `Person.Away` dims a face, `Person.Deactivated` greys it; `Photo` replaces the face in the same tile. `PresenceRow` puts the viewer first and outlined, shows eight faces and then "+n", marks editors with a pencil, and gives each face its name, tab count and state on hover and focus; it announces nothing by itself (MP-02, NFR-48.2). With `Shared: true` (and an `ID`) the row is the same for every viewer, as a shared view renders it: every face carries `data-who`, those after the eighth `data-overflow`, and the document's own style orders, outlines and reveals the viewer's face (`views.Presence`).

### Issues

```templ
@ui.TypeIcon(ui.Bug)                  // names itself, with a tooltip
@ui.TypeLabel(ui.Story)               // icon and name
@ui.PriorityIcon(ui.High)
@ui.StatusLozenge("In Review", ui.InProgress)
@ui.IssueChip(ui.IssueChipProps{Key: "WEB-12", Summary: s, Type: ui.Bug, Status: "In Review", Category: ui.InProgress, Href: "/WEB-12"})
@ui.Label("a11y", "/issues?label=a11y")
@ui.Due(due, now, overdue)
@ui.Points("3")
@ui.Progress(5, 8, "Board v1")
```

`IssueChip`, `Mention` and `PageLink` render the same elements as documents (`a.issue-chip[data-key]`, `a.mention[data-person]`, `a.page-link[data-page]`, see docs/editor.md), so one stylesheet serves both. A chip of a deleted or unknown issue says so (`State: ui.ChipDeleted`).

### Board

```templ
@ui.Board(ui.BoardProps{Label: "Board of Website"}) {
	@ui.Lane(ui.LaneProps{ID: "lane-review", Name: "In Review", Category: ui.InProgress, Count: 5, WIP: 4, CreateAttrs: createAttrs}) {
		@ui.Card(ui.CardProps{Key: "WEB-12", Summary: s, Href: "/WEB-12", Type: ui.Bug, Priority: ui.High,
			Assignee: &sunny, Labels: labels, Due: due, Now: now, Epic: "Board v1", Points: "3",
			State: ui.CardRefused, Note: "Sunny Otter moved WEB-12 to In Review first.", MenuID: "card-menu"})
		@ui.DropMarker()
	}
}
@ui.CardMenu(ui.CardMenuProps{ID: "card-menu", Lanes: lanes, Attrs: menuAttrs})
```

A card shows every part of FR-40.2. Its summary links the issue and the whole card takes the click; the grip is the touch handle; "…" opens the board's one `CardMenu` (Move to each lane, Move up, Move down, Open), the keyboard's way to move a card (FR-41.2). The menu's items carry `data-lane`, `data-move` and `data-open`, and the button that opened it carries `data-card`: one delegated handler on the menu reads both. `State` is `CardPending` (faded, the pixel walking), `CardRefused` (a shake, `Note` under it), `CardLanded` (a glow that cools), `CardPicked` (raised, keyboard) or `CardDragged` (a hole). A lane over its limit says so in words. `Lane.Target` lights the lane a drag is over. Lanes scroll sideways at 260px or more.

### Feedback

```templ
@ui.Notice(ui.NoticeProps{Tone: ui.ToneDanger, Live: true, Action: ui.Button(ui.ButtonProps{Label: "Use Critical anyway", Size: ui.SizeSM, Attrs: retryAttrs})}) {
	<p>Sunny Otter changed the priority to High 5 seconds ago. Your Critical was not saved.</p>
}
@ui.Outcome(ui.OutcomeProps{Kind: ui.OutcomeRefused, Text: "Move the 4 child issues first."})
@ui.Outcome(ui.OutcomeProps{Kind: ui.OutcomeFailed, RetryAttrs: retryAttrs})
@ui.Toasts(ui.ToastsProps{Toasts: toasts})
@ui.EmptyState(ui.EmptyProps{Icon: "board", Title: "Nothing on the board", Text: t, Action: create})
@ui.Skeleton("card", 3)
@ui.Stats([]ui.Stat{{Label: "Overdue", Value: "3", Tone: ui.ToneDanger}})
@ui.Feed("Notifications") { @ui.FeedItem(ui.FeedItemProps{Person: p, At: t, Now: now, Unread: true}) { … } }
@ui.Kbd("Ctrl", "K")
@ui.RelativeTime(t.In(zone), now)
```

The conflict of MP-08.3 is a danger `Notice` beside the field with the retry as its action; the person's value stays in the field. `Outcome` is a polite live region (a failure is an alert). `Toasts` is for news from elsewhere, never for the outcome of the person's own action. `RelativeTime` writes English text and the absolute date, time and zone in its title (NFR-51); the zone is `t`'s location. Its text is the server's at render time.

### Editor

```templ
<div class="tk-editor">
	<div class="tk-editor__bar">
		@ui.EditorToolbar("Formatting", ui.DefaultToolbar)
		@ui.EditorStatus(ui.Saving)
	</div>
	<sb-rich-editor …><div class="pm-mount prose" data-ignore-morph data-ignore></div></sb-rich-editor>
</div>
```

The toolbar is `role=toolbar` with one tab stop and Left and Right along it; every button has a name, a shortcut in `aria-keyshortcuts` and `aria-pressed` for marks and blocks (NFR-49). sb-rich-editor keeps its toolbar and status in its shadow root and follows these classes and tokens. Other editors' carets and selections are ProseMirror decorations in the light-DOM mount: `<span class="tk-caret" data-hue="7"><span class="tk-caret__tag">Sunny Otter</span></span>` (add `data-idle` to hide the tag, MP-06.3) and `<span class="tk-remote-sel" data-hue="7">`. Both are hidden from screen readers.

### Prose

`.prose` styles exactly the HTML of a document (docs/editor.md): `h2` to `h4` for headings, `pre[data-language] > code` with the language in a tab, `ul.task-list > li.task-item[data-checked]`, `figure.doc-image > img`, tables with a header row, the inline references, `span.link-unsafe`, and `ins.diff-ins` / `del.diff-del` for version compare (FR-88). `.prose--small` is for comments.

## Starbase components

`cmd/vendor-starbase` copies components from a Starbase checkout: it reads `static/vendor/starbase/pins.json`, copies each component's module and those of the components it renders, the fonts with their OFL texts and Starbase's MIT licence, and writes `lock.json` (the commit and each file's SHA-384) and `VERSION`. It refuses a checkout with uncommitted changes in what it copies, and one whose Datastar build is not tickit's. `TestVendored` checks the files against the lock.

```sh
go tool task vendor-starbase STARBASE=/develop/starbase
```

Vendored: `copy-button`, `date-picker`, `dropdown`, `popover`, `select`, `tabs`, `toast`, `tree`, `virtual-scroll`. The modules stay as Starbase wrote them; `edge.Static` minifies them in production. A page loads a component as a module after its import map:

```go
scripts, err := web.StarbaseScripts(files, "select", "tabs") // with the modules they render, in order
```

Not vendored, on purpose: dialogs and the side panel are native `<dialog>`s (sb-modal, sb-drawer), tooltips are CSS with an Escape hush (sb-tooltip), disclosures are `<details>` (sb-details), notices are markup (sb-alert), and sb-relative-time's title lacks the time zone NFR-51 asks for. Starbase's components read `--sb-*`, which `starbase.css` maps onto tickit's tokens, so themes and the smooth style reach them.

## Do and do not

- Do read semantic tokens. Do not use primitives or raw colours outside `tokens.css`.
- Do pass runtime values as data attributes. Do not write `style=`.
- Do give every type, priority and status its shape and name. Do not show colour alone.
- Do name every icon, as text beside it or as its label. Do not leave an icon-only control without `aria-label`.
- Do keep the focus ring. Do not set `outline: none` without a visible replacement.
- Do mark a sent control pending and end it with its outcome. Do not show a result before the server renders it.
- Do put a refusal next to its control. Do not put the outcome of the person's own action in a toast.
- Do keep Datastar expressions in Go constants that read ids from the element. Do not put a person's value in an expression.
- Do set data in the UI face. Do not set names, titles or documents in Pixelify Sans.
- Do put behaviour on a container and delegate (the card menu). Do not repeat an expression on a hundred cards.

## Adding an icon

1. Draw it in `internal/ui/sprites.go` as 16 rows of 16 characters: `#` a pixel, `+` a pixel in the second tone, `.` nothing. Keep a pixel of margin and two-pixel strokes where the shape allows.
2. Give it a plain name; add an alias in `aliases` if a tracker word fits better (`overdue` for `warning`).
3. Run `go test ./internal/ui`: a sprite of the wrong size fails at init. Look at it on `/design#icons` at 16 and 32 pixels, in both themes.

Issue type glyphs are in `typeGlyphs` (drawn in `--tk-hue-on` on the notched tile), priorities in `prioGlyphs` (each its own shape: `TestIcons` checks), status markers in `catMarkers` on a grid of 10.

## Testing what you build

- `TestContrast` for any colour change.
- `TestDesign` renders `/design` in three looks: no `style` attribute, `lang="en"`, the import map with the CSP nonce, the bundle with hashed fonts.
- Look at it: `go run ./cmd/tickit -dev -addr 127.0.0.1:18090`, then `/design?theme=light`, `?style=smooth`, and at 375 pixels wide.
