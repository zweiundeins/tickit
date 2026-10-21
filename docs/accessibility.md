# Accessibility checks

This page records how tickit is checked against NFR-45 to NFR-51 and section 12 item 8 of the requirements, what the checks found on 20 October 2026, and what is still open. Run the checks again after a change to `internal/ui`, the stylesheets or the editor.

## axe on the pages of NFR-45

`TestAxe` in `e2e/a11y_test.go` seeds the demo, opens the home page, the WEB board, WEB-2, the issue list, the page "Deploy to production" in view and in edit, search for "checkout" and the home page with the notification panel open, and runs axe-core on each, in the dark and the light theme. A serious or critical violation fails the test; moderate and minor ones are logged.

```sh
mkdir -p /tmp/axe && npm install --prefix /tmp/axe axe-core@4
TICKIT_AXE=/tmp/axe/node_modules/axe-core/axe.min.js CHROME=/usr/bin/chromium LITH_CHROME_NO_SANDBOX=1 go test ./e2e -run TestAxe -v
```

Without `TICKIT_AXE` the test skips. axe is injected through DevTools, so the page's CSP stays as it is.

With axe-core 4.14.0 the eight pages have no serious or critical violation in either theme. The fixes that got them there:

- Task list checkboxes in pages and descriptions had no name; they are named "Done", as in the editor.
- The menus and values of an issue's side column were named "Assignee" or "Edit the due date" while they show "Yuki Tanaka" or "Oct 27". Their names now hold what they show, "Assignee: Yuki Tanaka" (WCAG 2.5.3).
- The editor's heading buttons show H1 to H3 and were named "Heading 1" to "Heading 3"; they are "Heading H1" to "Heading H3".
- The bell was named "Notifications" beside a visible count; its name is now "Notifications 3".
- The first-visit hint is a labelled landmark, the notification panel's head no second banner, and the editor's page has an h1.

The same run by hand over the other pages (backlog, projects, spaces, a space, history, trash, saved filters, login, privacy, 404, a project's activity, the create dialog, the issue panel, the card menu, the display menu, the person menu, the search suggestions and `/design`) found no serious or critical violation after two styleguide fixes.

## What axe still reports

- `heading-order` (moderate) on wiki pages: a document's first heading is an h3 under the page's h1 when the page starts with a level 2 heading. The heading levels are the author's.

## Keyboard walk

A keyboard-only walk with DevTools key events, at 1280 by 900:

- **Creating an issue.** Tab from the top of the board reaches the skip link, the header, the board's tabs, its filters and the cards in reading order. Enter on Create opens the dialog with the focus in Summary; Tab moves through every field to Cancel and Create, and the modal dialog keeps the page behind it inert. Enter on Create opens the new issue in the panel; Escape closes it and the focus returns to Create.
- **Moving a card.** Enter on a card's "…" opens its menu with the focus on the first column; the arrow keys move through the columns and Enter moves the card. The focus stays on the card's button. Escape closes the menu and returns the focus to the button.
- **Bulk edit in the issue list** (21 October 2026, `TestBulkStatusLive`). Space on a row's box selects the issue and the bar's count, a polite live region, says "1 issue selected"; the head's box selects the page. Enter on "Set priority" opens its menu with the focus on the first value, Down moves to the next, Enter sends it for the selection. What became of it is a status line under the bar, one line per refused issue; the refused ones stay selected. axe finds nothing new on the list with the boxes and the bar.
- **Editing a page.** E on a page opens the editor (the key the Edit button shows did nothing before this check). The title field takes the focus, Tab reaches the insert buttons, the toolbar as one tab stop and the document.

Every focused control shows the 2px ring, except the editor's title, which shows a 2px line under it, and the document, which shows a bar beside it.

## Narrow screens and zoom

`TestNarrowPages` opens the same eight pages at 320 CSS pixels, 400 % of 1280, and fails when the page scrolls sideways. It needs no axe. A wider walk at 375 and 320 pixels also looked for content cut off by a clipping parent, which a page that does not scroll sideways can still have; the backlog, a page's linked issues, an issue's code blocks and comments, lane heads and tables were fixed. Tables and the board's lanes scroll inside their own region, as NFR-50 and NFR-56 allow.

## Reduced motion and forced colours

With `prefers-reduced-motion: reduce` no element on the board has a running transition or animation, and cards that others move jump to their place: the view transition's groups are covered too.

With `forced-colors: active` the board, an issue, a page and the issue list were checked by screenshot. Glyphs drawn as masks (types, priorities, status markers, faces, the card's "…", due dates) and page link icons vanished, since the system paints their fill in the canvas colour, and the task list's checkboxes lost their edges with their box shadows. They now keep their own colours or the text colour.

## Firefox and touch

On 20 October 2026 the whole e2e suite ran in Firefox 158.0.1 (Mozilla's linux-x86_64 tarball, headless, over WebDriver BiDi, without the content sandbox, which this machine's lack of user namespaces rules out):

```sh
LITH_BROWSER=firefox FIREFOX=/path/to/firefox/firefox LITH_FIREFOX_NO_SANDBOX=1 go test ./e2e
```

Every test that runs without a setting passed, `TestCoEditing` included (5 editors, keystroke to the others p50 63 ms, p95 100 ms), and so did `TestAxe` with `TICKIT_AXE` (no serious or critical finding; the same moderate heading-order note on a page as in Chromium). Firefox's driver had to learn five DevTools commands the tests send: `Input.insertText` (typed as key actions), `Network.getCookies` and `Network.setCookie` (BiDi's storage commands on the page's user context), `Network.setExtraHTTPHeaders` (`network.setExtraHeaders`), and `Emulation.setFocusEmulationEnabled`, which Firefox does not need. Two things are not covered in Firefox:

- `Emulation.setEmulatedMedia` has no BiDi counterpart, so `TestAxe` checks only the light theme in Firefox.
- `TestBoardTouchDrag` and `TestEditorTouch` skip: touch emulation (a coarse pointer, touch events) is a DevTools command that BiDi lacks.

Touch (NFR-56) is checked in Chromium with DevTools touch emulation, a 2x device and a coarse pointer:

- `TestBoardTouchDrag` (600 by 900 pixels): columns are at least 260 pixels wide in a board that scrolls sideways; a finger on a card's grip lifts it at once and drags it into the next column; the move is stored, so a second browser's board shows it; a finger on the card's title pans and moves nothing. A touch that lands while a view transition of the lanes runs (another viewer's card gliding in) is hit-tested on the board around them and pans instead of dragging, so the test waits for the card to be under the finger first.
- `TestEditorTouch` (390 by 844 pixels): a tap puts the caret in the page editor, typing and Enter work, the toolbar's "Bulleted list" makes a list, and the Link button's dialog takes an address by taps alone; the server's document has the list and the link.

## Not checked

- A screen reader. NFR-45.1 asks for a check with one; none ran here.
- WebKit (Safari, iOS) and real phones. The checks above ran in Chromium, the e2e suite also in Firefox (above); touch is emulated in Chromium only.
- Contrast is covered by `TestContrast` in `internal/web` and by axe's colour rules, not by eye.
