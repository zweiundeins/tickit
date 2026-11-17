# Changelog

tickit had no releases before v0.5.0. The versions below were tagged afterwards, on the commits where each stretch of work ended.

## v0.5.0 (2026-11-17)

- The issue's status, priority and type menus are Starbase's sb-dropdown with rich items: pixel icons and lozenges, a "Move to" group over the workflow's next statuses, a pick shown pending at once, and the server's value back on a refusal or conflict.
- The activity feeds use sb-virtual-scroll's own feed keys (Page Down and Up, Ctrl+End and Ctrl+Home), step-aligned windows, a row height from a design token and an unknown total until the last window; feed.js keeps only "Newer activity".
- Every Starbase element is written through Starbase's typed templ wrappers. The vendored components and the wrappers come from one release, Starbase v0.10.0, and a test checks that they match. A page that renders a component it does not load logs an error.
- The saved filter's toast links to Filters.
- Fixed: opening a menu no longer overwrites the version it was opened at, which could drop a change that arrived in that moment.
- lith v0.5.9.

## v0.4.0 (2026-11-14)

- New in pages and descriptions: headings to level 6, text colours, panels, status lozenges, tables of contents, dates, collapsible sections and tables of up to 500 rows. The seeded pages use them.
- An "On this page" outline (Starbase's sb-toc) beside a page with four headings or more.
- Heading ids carry their document, so a description and its comments never repeat one.
- Page limits per instance: the demo keeps 200 KB and 2,000 blocks, a private instance (`-limits private`) takes 1 MB and 10,000 blocks.
- Groundwork for importing Jira and Confluence: the mapping of Atlassian Document Format to tickit's schema (prosemirror-go's `adf`) and `tickit-adfcheck`, which reports what a sample would lose.
- Datastar 1.0.5 from lith instead of a vendored copy; Starbase v0.9.0 components.
- ProseMirror transform 1.12.5, prosemirror-go v0.2.1 and lith-coedit v0.2.1: undoing a mark change (a `markSet` step) is no longer refused.
- Fixed: a page trashed while someone edited it no longer stops the co-editing log from folding for every page (migrations 089 and 090).
- The fuzz targets of the notification and staff actions are back, and every fuzzer checks that a refusal changes nothing.
- lith v0.5.4 to v0.5.8, Go 1.27.4 and every dependency at its latest; the weekly race detector run has the time it needs.

## v0.3.0 (2026-10-29)

- Image uploads in pages and descriptions, bulk edit of up to 100 issues, inline comments on selected text, a workflow per project with board columns, WIP limits and swimlanes, and issues made from and listed in pages.
- Starbase components across the app: pickers, date pickers, copy buttons, toasts, tabs, the page tree, the admin menu and virtual-scroll feeds.
- The page tree loads its branches as they open, and the sidebar is one element on every screen width.
- The document model and co-editing moved into their own modules, github.com/zweiundeins/prosemirror-go and github.com/zweiundeins/lith-coedit.
- Chips and comments of an editor are shared views, so a commit renders them once for all editors.
- Every action names its tab and op in the Tab-Id and Op-Id headers; rate limits and failed writes come as notices, and a page offers a reload after a deploy.
- lith v0.4.2 to v0.5.3.

## v0.2.0 (2026-10-20)

The rewrite on lith, deployed as the public demo at tickit.zweiundeins.gmbh in place of the first codebase.

- Projects, issues, boards, backlogs, sprints, lists with filters, search, dashboards and notifications.
- Spaces and pages with a page tree, versions and comments, co-edited live with carets.
- Presence, typing indicators and conflicts in words, in every tab at once.
- The public demo: guests with pseudonyms, a seeded company, a nightly reset, caps and moderation.
- A design system of its own, with pixel icons, light and dark.

## v0.1.0 (2026-10-12)

The first codebase, started from the northstar template: Go with an embedded NATS, CQRS and event sourcing, issues and projects with generated keys, user management with sessions and roles, notifications, and TLS with HTTP/2, HTTP/3 and brotli.
