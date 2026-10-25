# Requirement status

Where the rewrite stands on 20 October 2026, read from the code on lith v0.3.0; the performance rows are measured on lith v0.4.1. It covers the 160 Must and Should requirements of docs/requirements.md (Could and Won't for now are left out) and judges each against the code today, not against the "Today:" line of the requirements, which describes the old codebase. "Done" means every acceptance criterion is implemented; "partly" that some are, and the note says which are missing. The Tests column names the tests that cite the requirement, or that prove it where none cites it. Tests in `e2e/` and `editor/test/run.mjs` drive a browser and need Chromium; `editor/test/` runs with `node test/run.mjs` in `editor/`, not with `go test`. The performance requirements are judged from the load test `cmd/tickit-load` (`go tool task load`), whose numbers are in benchmarks/RESULTS.md. Update a row with the change that finishes or breaks its requirement.

## Summary

| Area | Must done | Must partly | Must not done | Should done | Should partly | Should not done |
| --- | --- | --- | --- | --- | --- | --- |
| Projects | 1 | 0 | 0 | 1 | 0 | 0 |
| Issues | 9 | 0 | 0 | 8 | 0 | 0 |
| Workflows and statuses | 1 | 0 | 0 | 1 | 0 | 0 |
| Boards, backlog and sprints | 2 | 0 | 0 | 2 | 0 | 0 |
| Search, filters, bulk edit and dashboards | 7 | 0 | 0 | 2 | 0 | 0 |
| Comments, mentions, activity and history | 2 | 0 | 0 | 4 | 0 | 0 |
| Spaces and pages | 7 | 0 | 0 | 8 | 0 | 0 |
| Links between pages and issues | 2 | 0 | 0 | 2 | 0 | 0 |
| Notifications | 4 | 0 | 0 | 3 | 0 | 0 |
| Identity, accounts and sign-in | 5 | 0 | 0 | 1 | 0 | 0 |
| Administration | 0 | 0 | 0 | 3 | 0 | 0 |
| Import, export and API | 0 | 0 | 0 | 0 | 0 | 0 |
| Audit | 1 | 0 | 0 | 1 | 0 | 0 |
| Multiplayer | 10 | 0 | 0 | 3 | 0 | 0 |
| Demo operation | 8 | 0 | 0 | 1 | 0 | 0 |
| Non-functional: Security | 17 | 0 | 0 | 3 | 0 | 0 |
| Non-functional: Privacy | 4 | 2 | 0 | 1 | 0 | 0 |
| Non-functional: Performance and realtime | 4 | 4 | 0 | 1 | 1 | 0 |
| Non-functional: Accessibility | 5 | 0 | 0 | 1 | 1 | 0 |
| Non-functional: Browsers | 0 | 1 | 0 | 1 | 0 | 0 |
| Non-functional: Deployment on the shared host | 8 | 0 | 0 | 0 | 0 | 0 |
| Non-functional: Backups | 2 | 0 | 0 | 1 | 0 | 0 |
| Non-functional: Observability | 3 | 0 | 0 | 1 | 0 | 0 |
| **All** | **102** | **7** | **0** | **49** | **2** | **0** |

## Projects

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-01 | Project list and keys | Must | done | internal/tracker/commands_test.go: TestProjects; internal/web/board_test.go: TestCreateProject; internal/tracker/commands_test.go: TestKeys | The project page has Board, Backlog, Issues and Activity tabs; FR-01.2 has no test of its own. |
| FR-02 | Create a project | Should | done | internal/tracker/commands_test.go: TestProjects; internal/web/board_test.go: TestCreateProject; internal/access/access_test.go: TestScopes; internal/tracker/decide_test.go: TestProjectDeciders |  |

## Issues

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-10 | Create an issue | Must | done | internal/tracker/decide_test.go: TestCreateIssue; internal/web/issue_actions_test.go: TestCreateIssue; e2e/board_test.go: TestBoardPanelAndCreate (Chromium); e2e/pickers_test.go: TestCreatePickAssignee (Chromium) | The live appearance on other open boards and lists (FR-10.3) is covered only by the general MP-03 tests. |
| FR-11 | Issue numbers and keys | Must | done | internal/tracker/commands_test.go: TestKeys; internal/tracker/commands_test.go: TestNumbersUnderConcurrency; internal/tracker/decide_test.go: TestProjectDeciders |  |
| FR-12 | Open an issue by key | Must | done | internal/tracker/commands_test.go: TestKeys; internal/web/issue_actions_test.go: TestIssueURLs |  |
| FR-13 | View an issue | Must | done | e2e/issue_test.go: TestIssuePanel (Chromium); e2e/issue_test.go: TestIssueTwoPeople (Chromium) | No test checks every field of FR-13.1, but the issue page renders all of them with names and faces. |
| FR-14 | Edit summary and description | Must | done | e2e/issue_test.go: TestIssueEditInPlace (Chromium); internal/tracker/decide_test.go: TestEditIssue; internal/tracker/commands_test.go: TestFieldConflicts; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/issue_actions_test.go: TestIssueActionsRefused |  |
| FR-15 | Rich-text description | Should | done | internal/tracker/description_test.go: TestDescriptionCommits; internal/tracker/description_test.go: TestDescriptionVersions; internal/tracker/description_test.go: TestDescriptionFold; internal/tracker/description_test.go: TestDescriptionReset; internal/tracker/decide_test.go: TestCutDescription; internal/notify/notify_test.go: TestMentions; internal/activity/activity_test.go: TestFeeds; internal/web/desc_test.go: TestDescriptionLive; internal/web/desc_test.go: TestDescriptionAnswers; e2e/desc_coedit_test.go: TestDescriptionCoEditing (Chromium) | The description is a co-edited document in the page editor (docs/editor.md, "Issue descriptions"), whose versions are cut as issue.description_changed; the create dialog still takes text markup. In the issue panel, Edit opens the issue's page with the editor. |
| FR-16 | Change status | Must | done | internal/tracker/issuepage_test.go: TestProjectWorkflow; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/board_test.go: TestBoardMoveAccepted; internal/tracker/bulk_test.go: TestBulkEdit; internal/web/bulk_test.go: TestBulkStatus; e2e/bulk_test.go: TestBulkStatusLive (Chromium) | Status changes from the menu, the board and the issue list's bulk edit (FR-58) work live; a bulk edit runs each issue's change through the menu's command, so the workflow's rules apply to it too. |
| FR-17 | Assign and unassign | Must | done | internal/tracker/commands_test.go: TestAssign; internal/tracker/decide_test.go: TestEditIssue; internal/notify/notify_test.go: TestAssignment; e2e/pickers_test.go: TestPickAssigneeByTyping (Chromium) |  |
| FR-18 | Change priority | Must | done | internal/ui/ui_test.go: TestIcons; internal/web/issue_actions_test.go: TestIssueActionsAccepted |  |
| FR-19 | Issue types | Must | done | internal/tracker/commands_test.go: TestEpicsAndSubtasks; internal/tracker/decide_test.go: TestEditIssue; internal/web/issue_actions_test.go: TestIssueActionsRefused |  |
| FR-20 | Epics and their issues | Should | done | internal/tracker/commands_test.go: TestEpicsAndSubtasks |  |
| FR-21 | Subtasks | Should | done | internal/tracker/commands_test.go: TestEpicsAndSubtasks; internal/web/issue_actions_test.go: TestIssueActionsAccepted |  |
| FR-22 | Issue links | Should | done | internal/tracker/commands_test.go: TestLinks; internal/tracker/decide_test.go: TestSetDeciders; internal/web/issue_actions_test.go: TestIssueActionsRefused |  |
| FR-23 | Labels on issues | Should | done | internal/tracker/search_test.go: TestLabels; internal/tracker/decide_test.go: TestCreateIssue; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/ux_test.go: TestLabelsAndOverdue | Labels on cards, in the issue list (beside the summary) and on the issue page link to the list of their issues (FR-23.2). |
| FR-25 | Due date | Should | done | internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/ux_test.go: TestLabelsAndOverdue; e2e/pickers_test.go: TestDueDateByKeyboard (Chromium) | An overdue issue says "Overdue" in visible text beside the warning icon on cards, in lists and on the home page (FR-25.2). |
| FR-27 | Watchers | Should | done | internal/tracker/decide_test.go: TestCreateIssue; internal/tracker/decide_test.go: TestEditIssue; internal/tracker/commands_test.go: TestComments; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/notify/notify_test.go: TestWatchers |  |
| FR-28 | Delete an issue | Should | done | internal/tracker/commands_test.go: TestKeys; internal/web/issue_actions_test.go: TestIssueURLs | No test checks that a deleted issue leaves open boards and lists in the next frame (FR-28.1). |

## Workflows and statuses

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-35 | Default workflow | Must | done | internal/tracker/commands_test.go: TestProjects; internal/tracker/decide_test.go: TestCreateIssue; internal/tracker/search_test.go: TestSearchSorts; internal/tracker/workflow_test.go: TestWorkflowCommands | Every project starts with the default workflow and board, and the seed keeps it; an Admin may then change both (FR-36, FR-37 and FR-43, Coulds, are built: internal/tracker/workflow_test.go, internal/web/workflow_test.go: TestWorkflowLive, e2e/workflow_test.go: TestWorkflowRuleLive (Chromium)). The seed gives OPS one rule: Done follows In Review only. Lists, menus, the board, dashboards, the history, feeds and notifications read the project's workflow. |
| FR-38 | Resolution time | Should | done | internal/tracker/commands_test.go: TestBoardAndBacklog; internal/web/ux_test.go: TestListResolved; internal/web/dashboards_test.go: TestDashboards | The resolved time is set, cleared and shown on the issue page; the issue list has a Resolved column, sorts and filters by it (resolved_from, resolved_to), and dashboards chart created against resolved per day. A status whose category an Admin changes moves the resolved time of the issues that hold it (internal/tracker/workflow_test.go: TestWorkflowCommands). |

## Boards, backlog and sprints

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-40 | Kanban board | Must | done | internal/tracker/commands_test.go: TestBoardAndBacklog; e2e/board_test.go: TestBoardPanelAndCreate (Chromium); e2e/board_test.go: TestBoardPresence (Chromium) | The render budget for 200 issues in FR-40.4 depends on NFR-38 (see benchmarks). The columns are the project's board columns (FR-43), one per status but Backlog by default; swimlanes by epic or assignee per tab (FR-44): internal/web/workflow_test.go: TestBoardSwimlanes, e2e/workflow_test.go: TestBoardSwimlanesTab (Chromium). |
| FR-41 | Move cards | Must | done | internal/web/board_test.go: TestBoardMoveAccepted; internal/web/board_test.go: TestBoardMoveRefused; internal/web/board_test.go: TestBoardMoveForbidden; internal/tracker/decide_test.go: TestMoveDecider; internal/tracker/rank_test.go: TestRanksStayOrdered; e2e/board_test.go: TestBoardMoveLive (Chromium); e2e/board_test.go: TestBoardKeyboardMove (Chromium); e2e/board_test.go: TestBoardMoveConflict (Chromium); internal/tracker/workflow_test.go: TestMoveToColumn; e2e/workflow_test.go: TestWorkflowRuleLive (Chromium) | A move the workflow's rules forbid is refused with the rule in words ("Done can only follow In Review."); while dragging, the columns a card may not go to are dimmed, and the card menu lists only the allowed ones. |
| FR-42 | Board quick filters | Should | done | e2e/board_test.go: TestBoardQuickFilter (Chromium); internal/tracker/commands_test.go: TestBoardAndBacklog |  |
| FR-45 | Backlog | Should | done | e2e/board_test.go: TestBacklogRank (Chromium); internal/tracker/commands_test.go: TestBoardAndBacklog; internal/web/board_test.go: TestSprints |  |

## Search, filters, bulk edit and dashboards

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-50 | Issue list | Must | done | internal/web/list_test.go: TestListFilters; e2e/list_test.go: TestListPanel (Chromium) |  |
| FR-51 | Filter issues | Must | done | internal/tracker/search_test.go: TestSearchFilters; internal/web/list_test.go: TestListFilters; e2e/pickers_test.go: TestListPickers (Chromium) | A changed filter clears the page number in the page's script (FR-51.3), which no test drives. |
| FR-52 | Text search in issues | Must | done | internal/tracker/search_test.go: TestSearchFilters; internal/web/list_test.go: TestListFilters; internal/web/list_test.go: TestListChange; internal/search/search_test.go: TestRanking; internal/web/search_test.go: TestSearchSuggest; e2e/list_test.go: TestListTypingAndHistory (Chromium) |  |
| FR-53 | Sort issues | Must | done | internal/tracker/search_test.go: TestSearchSorts; internal/web/list_test.go: TestListSortAndPages; internal/web/list_test.go: TestListFilters |  |
| FR-54 | Pages of results | Must | done | internal/tracker/search_test.go: TestSearchPages; internal/web/list_test.go: TestListSortAndPages | The list view renders again on every issue change; no test moves issues across a page boundary (FR-54.2). |
| FR-55 | Filter, sort and page in the URL and the tab | Must | done | internal/tracker/search_test.go: TestFilterValues; internal/web/list_test.go: TestListURL; internal/web/list_test.go: TestListChange; e2e/list_test.go: TestListTypingAndHistory (Chromium) |  |
| FR-56 | Saved filters | Should | done | internal/tracker/search_test.go: TestSavedFilters; internal/web/list_test.go: TestSavedFilters; internal/web/ux_test.go: TestBoardSavedFilter; e2e/ux_test.go: TestTrackerGaps (Chromium); internal/web/dashboards_test.go: TestDashboards | Save, star, the Filters menu, rename and delete work; a saved filter feeds the board's quick filter (a Filters menu above the board, `filter=` in its URL) and a dashboard gadget and an issue list on a page (FR-56.2, FR-104). |
| FR-57 | Search across pages and issues | Must | done | internal/search/search_test.go: TestRanking; internal/search/search_test.go: TestSnippet; internal/search/search_test.go: TestFilters; internal/web/search_test.go: TestSearchPage; internal/web/search_test.go: TestSearchSuggest; internal/wiki/commands_test.go: TestPageSearch; e2e/list_test.go: TestSearchBox (Chromium) |  |
| FR-59 | Home page with your work | Should | done | internal/web/notify_test.go: TestHomeAndReactions; internal/web/ux_test.go: TestRecentlyViewed | Assigned issues, "recently edited by you", "Recently viewed" (the issue and page views post once as they open; the newest 20 per person, swept after 30 days) and the live feed (FR-59.1). |

## Comments, mentions, activity and history

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-65 | Comment on an issue | Must | done | internal/tracker/commands_test.go: TestComments; internal/tracker/decide_test.go: TestCommentDeciders; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/tracker/issuepage_test.go: TestTyping | The per-tab draft lives in sessionStorage; no test checks it across a reconnect. |
| FR-66 | Edit and delete own comments | Should | done | internal/tracker/commands_test.go: TestComments; internal/web/issue_actions_test.go: TestIssueActionsAccepted | TestComments cites "FR-65 to FR-67" and covers edit, delete, "Comment deleted" and a moderator's hide. |
| FR-67 | Mentions | Should | done | internal/tracker/commands_test.go: TestComments; internal/tracker/decide_test.go: TestCommentDeciders; internal/notify/notify_test.go: TestMentions; internal/web/ux_test.go: TestMentionsAndPersonPage; e2e/ux_test.go: TestTrackerGaps (Chromium) | A mention shows the person's face and name and links to /people/<id>: their face, name, issues assigned and reported, and what they did lately (FR-67.2). Drafts and texts to edit show names, not ids. |
| FR-68 | Issue history | Must | done | internal/tracker/commands_test.go: TestHistory; internal/tracker/description_test.go: TestDescriptionVersions; internal/web/desc_test.go: TestDescriptionLive | A description's versions are entries with a comparison to the version before. |
| FR-69 | Activity feed | Should | done | internal/activity/activity_test.go: TestFeeds; internal/activity/activity_test.go: TestPageEditsMerge; internal/web/notify_test.go: TestHomeAndReactions; internal/web/feed_test.go: TestFeedWindows; e2e/feed_test.go: TestHomeFeedScrolls (Chromium) | The feeds of the home page, a project and a space scroll 1,000 entries deep in sb-virtual-scroll, a window of 50 a frame. |
| FR-70 | Issue or page as of an earlier version | Should | done | internal/tracker/commands_test.go: TestHistory | Sliders exist for issues and pages; tests cover only the issue's state at a version, not the slider or the shared render (FR-70.3). |

## Spaces and pages

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-75 | Spaces and their home pages | Must | done | internal/web/wiki_test.go: TestWikiPages; internal/demo/seed/content_test.go: TestPageCounts |  |
| FR-76 | Create a space | Should | done | internal/wiki/commands_test.go: TestCreateSpace; internal/web/wiki_test.go: TestWikiPages |  |
| FR-78 | Page tree | Must | done | internal/wiki/commands_test.go: TestMoves; internal/web/wiki_test.go: TestWikiLive; e2e/sbnav_test.go: TestPageTree (Chromium); internal/web/wiki_test.go: TestPageTreeLazy | Starbase's sb-tree on the server's lists; branches off the current page's path come when a tab opens them, so a space of 400 pages sends a tree of 28 to 73 KB rather than 273 to 281 KB. On a narrow screen the menu panel takes the sidebar's one copy while it is open (e2e/sbnav_test.go: TestPageTreeInMenuPanel). Without script, the tree shows the current page's path and the home page's children only (closed branches were hidden before too). |
| FR-79 | Move and reorder pages | Should | done | internal/wiki/commands_test.go: TestMoves; e2e/sbnav_test.go: TestMoveDialogTree (Chromium) | Moves go through a Move dialog that picks the place in a tree of the spaces and their pages; the page tree has no drag. |
| FR-80 | View a page | Must | done | internal/web/wiki_test.go: TestWikiPages; internal/web/wiki_test.go: TestWikiLive; e2e/editor_reconnect_test.go: TestReaderSeesEdits (Chromium) |  |
| FR-81 | Create a page | Must | done | internal/web/wiki_test.go: TestWikiPages; internal/wiki/commands_test.go: TestPageTitles; internal/web/wiki_test.go: TestWikiLive |  |
| FR-82 | Rich-text editor | Must | done | editor/test/run.mjs: "input rules and shortcuts (FR-82.2)" (Chromium); editor/test/run.mjs: "paste keeps supported formatting only and offers chips and page links (FR-82.3, DM-06)" (Chromium); editor/test/run.mjs: "undo takes back only the own changes (MP-05.7, FR-82.4)" (Chromium); editor/test/run.mjs: "a task checkbox toggles with one attr step; others see it (FR-82.1)" (Chromium); prosemirror-go fuzz_test.go: TestRebaseGenerated | The editor's browser tests run with `node test/run.mjs` in editor/, not with go test. |
| FR-83 | Tables in pages | Should | done | internal/docnodes/rules_test.go: TestRulesRefuse; editor/test/run.mjs: "limits: input stops at 200 KB and at 50 table rows, and says why (DM-05)" (Chromium) | The table toolbar (rows, columns, header row) exists; no test drives it. |
| FR-84 | Images from the library | Should | done | internal/demo/seed/content_test.go: TestPeopleImagesTemplates; internal/docnodes/rules_test.go: TestRulesRefuse; internal/web/wiki_test.go: TestWikiPages |  |
| FR-86 | Edit a page together | Must | done | internal/wiki/commands_test.go: TestPageTitles; internal/web/wiki_test.go: TestWikiLive; e2e/editor_coedit_test.go: TestCoEditing (Chromium) |  |
| FR-87 | Page versions | Must | done | internal/wiki/commands_test.go: TestVersions; internal/web/wiki_test.go: TestWikiPages |  |
| FR-88 | Compare versions | Should | done | internal/docnodes/diff_test.go: TestDiff |  |
| FR-89 | Restore a version | Should | done | internal/wiki/commands_test.go: TestVersions; prosemirror-go diff_test.go: TestRestoreStep |  |
| FR-90 | Page comments | Should | done | internal/wiki/commands_test.go: TestComments; internal/web/wiki_test.go: TestPageCommentEdit | Comments, replies, delete and hide work; the author edits their own comment in a dialog (`POST /cmd/pages/comment/edit`, plain text as when adding), and it then says "edited". FR-91 (a Could) is built too: inline comments on a range of text, live in readers and editors, detached when their text goes, resolved and reopened (internal/wiki/inline_test.go, internal/docnodes/highlight_test.go, internal/web/inline_test.go: TestInlineCommentsLive, internal/notify/wiki_test.go: TestInlineCommentNotifies, e2e/inline_test.go: TestInlineCommentsCoEditing, TestInlineCommentsReading (Chromium)). |
| FR-94 | Page trash | Should | done | internal/wiki/commands_test.go: TestTrash |  |

## Links between pages and issues

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-100 | Issue chips on pages | Must | done | internal/wiki/commands_test.go: TestChips; internal/web/wiki_test.go: TestChipsLive; internal/docnodes/rules_test.go: TestRulesRefuse; e2e/editor_reconnect_test.go: TestReaderSeesEdits (Chromium); editor/test/run.mjs: "input rules and shortcuts (FR-82.2)" (Chromium) |  |
| FR-101 | Pages that mention an issue | Must | done | internal/wiki/commands_test.go: TestChips; internal/web/wiki_test.go: TestChipsLive |  |
| FR-102 | Link a page from an issue | Should | done | internal/web/ux_test.go: TestLinkPage; internal/tracker/pagelinks_test.go: TestPageLinkDecider; internal/tracker/rebuild_test.go: TestRebuild; e2e/ux_test.go: TestTrackerGaps (Chromium) | "Link page" searches pages by title and links one with issue.page_linked on the issue's stream (projection issue_pages); the issue lists it beside the pages that mention it, the page under "Linked issues", both live. |
| FR-103 | Links between pages | Should | done | internal/docnodes/rules_test.go: TestRulesRefuse | Page links name the page id and "Linked from" is listed; no test covers the page picker or the "Linked from" list. |
| FR-104 | Issue list on a page | Could | done | internal/docnodes/issuelist_test.go: TestIssueListRules, TestIssueListHTML; internal/web/embed_test.go: TestIssueListOnPage; editor/test/run.mjs: "an issue list" (Chromium); e2e/embed_test.go: TestIssueListAndCreateFromPage (Chromium) | An `issue_list` block holds a saved filter or a filter in the issue list's query syntax and a cap of 1 to 25 rows. "Me" in its filter is the page's creator. Versions and compare show the filter, not rows. |
| FR-105 | Create an issue from a page | Could | done | internal/web/embed_test.go: TestCreateIssueFromPage; internal/docnodes/issuelist_test.go: TestFindInBlock; editor/test/run.mjs: "create issue" (Chromium); e2e/embed_test.go: TestIssueListAndCreateFromPage (Chromium) | The new issue is a task. The project defaults to the one whose key is the space's key, since spaces have no linked project, else the person's last. A selection that spans paragraphs in view mode creates and links the issue without a chip. |

## Notifications

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-110 | Notification panel | Must | done | internal/web/notify_test.go: TestBellAndActions; e2e/notify_test.go: TestBellLive (Chromium) |  |
| FR-111 | Notify on assignment | Must | done | internal/notify/notify_test.go: TestAssignment; internal/web/notify_test.go: TestHomeAndReactions |  |
| FR-112 | Notify on mention | Should | done | internal/notify/notify_test.go: TestMentions; internal/notify/wiki_test.go: TestPageNotifications |  |
| FR-113 | Notify watchers | Should | done | internal/notify/notify_test.go: TestWatchers |  |
| FR-115 | Mark read and dismiss | Must | done | internal/notify/notify_test.go: TestReadAndDismiss; internal/web/notify_test.go: TestBellAndActions; e2e/notify_test.go: TestBellLive (Chromium) |  |
| FR-116 | No notifications for views and restarts | Must | done | internal/notify/notify_test.go: TestReplayAndRestart; internal/web/action_test.go: TestGetNeverWrites | Criterion 1 is proven by the GET-never-writes probe test, which does not cite FR-116. |
| FR-117 | Notification retention | Should | done | internal/notify/notify_test.go: TestSweep | The sweep runs hourly, not daily, which meets the criterion. |

## Identity, accounts and sign-in

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-120 | Guest identity | Must | done | internal/people/people_test.go: TestGuestIdentity; internal/web/stream_test.go: TestPageAndStream |  |
| FR-122 | Log in | Must | done | internal/people/people_test.go: TestLoginLogout; internal/people/people_test.go: TestLoginEnumeration; internal/people/people_test.go: TestLoginStates; internal/web/action_test.go: TestLogin; e2e/login_test.go: TestLogin (Chromium) |  |
| FR-123 | Log out | Must | done | internal/people/people_test.go: TestLoginLogout; internal/web/action_test.go: TestLogin; e2e/login_test.go: TestLogin (Chromium) |  |
| FR-124 | Login sessions expire | Must | done | internal/people/people_test.go: TestLoginLogout; internal/web/demo_test.go: TestAccounts | Expiry (flag `-session-ttl`), password change and deactivation end sessions; password reset does not exist (FR-128 is Could), and no test asserts that a password change ends the other sessions. |
| FR-125 | Change password | Should | done | internal/web/demo_test.go: TestAccounts | `/account` asks for the current password and the new one twice; ending the other sessions is in `people.ChangePassword` but no test asserts it. |
| FR-137 | Header and navigation | Must | done | internal/web/action_test.go: TestLogin; internal/web/action_test.go: TestAdminLink; internal/web/dashboards_test.go: TestDashboards; e2e/ux_test.go: TestTrackerGaps (Chromium) | The header's Dashboards link opens /dashboards; below 30rem the search is an icon button that opens the field. Actions a person may not use are not offered (Delete issue, TestDeleteOfferedToThoseWhoMay). |

## Administration

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-140 | Account list | Should | done | internal/web/demo_test.go: TestStaffOnly | No test cites it; `/admin` lists accounts 50 a page with search, role and status filters and last login, and `/admin/guests` the guests of 24 hours; tests check only the 403s and screenshots (e2e/demo_test.go: TestDemoShots, Chromium, needs TICKIT_SHOTS). |
| FR-142 | Assign site roles | Should | done | internal/web/demo_test.go: TestAccounts; e2e/sbnav_test.go: TestAdminRowMenu (Chromium) | The accounts share one row menu (sb-context-menu). |
| FR-143 | Deactivate and reactivate accounts | Should | done | internal/web/demo_test.go: TestAccounts | Reactivation restoring login is implemented in `people.SetActive` but not tested. |

## Import, export and API

No Must or Should requirements.

## Audit

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| FR-155 | Every change has an author and a time | Must | done | internal/tracker/rebuild_test.go: TestRebuild; internal/wiki/rebuild_test.go: TestRebuild; internal/demo/reset_test.go: TestReset |  |
| FR-156 | Security log | Should | done | internal/people/people_test.go: TestLoginLogout; internal/web/demo_test.go: TestAccounts | No test cites it; logins (failed too), logouts, password and role changes, deactivations and moderation are logged without secrets, filterable on `/admin/security`; the 90-day deletion in `people.Sweep` is not tested. |

## Multiplayer

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| MP-01 | One identity across tabs | Must | done | internal/web/stream_test.go: TestPresence; internal/views/views_test.go: TestPresenceRow; internal/web/notify_test.go: TestBellAndActions; e2e/notify_test.go: TestBellLive (Chromium) |  |
| MP-02 | Presence | Must | done | internal/web/stream_test.go: TestPresence; internal/web/mp_test.go: TestPresenceGoes; internal/web/mp_test.go: TestAway; internal/web/mp_test.go: TestSpaceTreePresence; internal/web/presence_rows_test.go: TestHomeAndDashboardPresence; internal/ui/ui_test.go: TestPresenceOrder; internal/views/views_test.go: TestPresenceRow; e2e/presence_test.go: TestPresenceRow (Chromium); e2e/board_test.go: TestBoardPresence (Chromium) | A connection that drops without closing ends once its page stops answering heartbeats (`POST /alive`, 50 s); the hub's heartbeat and write deadline alone kept such a face for minutes, since the writes go into the kernel's buffers. Behind Caddy too, since the answer comes from the page. Away is per person: no input in any tab for 5 minutes. |
| MP-03 | Live updates everywhere | Must | done | internal/web/list_test.go: TestListLive; internal/web/search_test.go: TestSearchLive; internal/search/search_test.go: TestFollowsChanges; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/wiki_test.go: TestWikiLive; internal/web/wiki_test.go: TestChipsLive; e2e/issue_test.go: TestIssueTwoPeople (Chromium); e2e/issue_test.go: TestIssueEditInPlace (Chromium); e2e/frames_test.go: TestFramesKeepPlace (Chromium); e2e/frames_test.go: TestDashboardKeepsPlace (Chromium); e2e/feed_test.go: TestHomeFeedScrolls (Chromium) | Lists, search, issues, boards, trees, chips, dashboards and the bell update live, and frames keep focus, fields and the place on screen (MP-03.3: the page holds the focused element where it was when a frame changes the layout above it, which the browser's scroll anchoring does not). |
| MP-04 | Live board moves | Must | done | internal/web/board_test.go: TestBoardMoveAccepted; internal/web/board_test.go: TestBoardMoveRefused; internal/tracker/commands_test.go: TestMoveConflicts; internal/tracker/decide_test.go: TestMoveDecider; e2e/board_test.go: TestBoardMoveLive (Chromium); e2e/board_test.go: TestBoardMoveConflict (Chromium) |  |
| MP-05 | Co-editing a page without losing edits | Must | done | internal/wiki/commands_test.go: TestCommits; internal/wiki/commands_test.go: TestConcurrentCommits; e2e/editor_coedit_test.go: TestCoEditing (Chromium); e2e/editor_load_test.go: TestCoEditingLoad; e2e/editor_reconnect_test.go: TestEditorReconnect (Chromium); lith-coedit editor/test/unit/collab.test.js: 'typing only: every character survives, in each editor order'; lith-coedit editor/test/unit/collab.test.js: 'undo takes back only the own steps, rebased'; editor/test/run.mjs: 'morph: frames, a morph around the editor and a signal patch change nothing in it (Appendix B done 3, MP-05.8)'; editor/test/run.mjs: 'skipped frames: with 60 % of frames held back and overtaken, only timing changes (MP-05.9)' (Chromium); e2e/editor_mp_test.go: TestEditorGapCopy (Chromium); e2e/delay_test.go: TestDelayProxy | MP-05.3 with a 50 ms round trip (a delaying proxy, `COEDIT_DELAY=25ms`): 20 browser editors at 5 characters a second, p95 140 to 148 ms and identical within 179 ms of the last keystroke on a quiet machine; p95 336 ms and more while other jobs saturated its CPUs (benchmarks/RESULTS.md). After a gap the editor cannot merge, the unsaved text shows as a copy beside the page and stays in the tab (MP-05.6). |
| MP-06 | Other people's carets and selections | Should | done | editor/test/run.mjs: 'remote carets: bar and name in the person\'s hue, at their place, hidden from screen readers (MP-06)' (Chromium) | The 30 s idle state is set in internal/web/routes_editor.go but no Go test cites MP-06. |
| MP-07 | Editing and typing indicators | Should | done | internal/tracker/issuepage_test.go: TestTyping; internal/web/mp_test.go: TestTypingGoesWithTab; internal/web/wiki_test.go: TestWikiLive; e2e/issue_test.go: TestIssueTwoPeople (Chromium); e2e/issue_test.go: TestIssueEditInPlace (Chromium); e2e/editor_reconnect_test.go: TestReaderSeesEdits (Chromium) | An indicator goes at once when its field is saved, cancelled or left (the comment box's blur) and when its tab's stream ends, unless a stream with its ID replaced it. |
| MP-08 | Conflicts on issue fields | Must | done | internal/tracker/commands_test.go: TestFieldConflicts; internal/tracker/decide_test.go: TestEditConflicts; internal/web/issue_actions_test.go: TestIssueConflict; internal/tabs/tabs_test.go: TestRefusalData; e2e/issue_test.go: TestIssueTwoPeople (Chromium) |  |
| MP-09 | Pending state and refusals | Must | done | internal/tabs/tabs_test.go: TestAccepted; internal/tabs/tabs_test.go: TestRefused; internal/web/stream_test.go: TestOutcomesOnTheStream; internal/web/issue_actions_test.go: TestIssueActionsAccepted; internal/web/issue_actions_test.go: TestIssueActionsRefused; e2e/issue_test.go: TestIssueEditInPlace (Chromium); e2e/home_test.go: TestHome (Chromium); e2e/ops_test.go: TestOpsEnd (Chromium); internal/web/action_test.go: TestRateLimits | A request without an outcome ends on its notice (a rate limit, a failed write, a reset) or when the stream connects again without it (`static/ops.js`, lith v0.5.0), never on a timer; TestOpsEnd drives both on the person menu's rename. The issue page's "This did not go through" with Retry (MP-09.3) has no test of its own. |
| MP-10 | Reconnect and catch-up | Must | done | internal/web/stream_test.go: TestReconnect; internal/web/stream_test.go: TestWatch; e2e/editor_reconnect_test.go: TestEditorReconnect (Chromium); editor/test/run.mjs: 'reconnect: unconfirmed steps go out again with the same ref and apply once (MP-05.6, MP-10.3)' (Chromium); e2e/reconnecting_test.go: TestReconnectingNotice (Chromium) | "Reconnecting" shows as soon as the stream's request fails and hides with the new stream's first frame (TestReconnectingNotice; until 23 October the heartbeat's effect hid it at once, so it never showed). The 50 s watchdog after a silent drop has no test. |
| MP-11 | One render per shared view | Must | done | internal/web/list_test.go: TestListOneRender; internal/web/board_test.go: TestBoardOneRenderPerTick; internal/web/board_load_test.go: TestBoardLoad; internal/tracker/search_test.go: TestFilterValues; internal/web/mp_test.go: TestViewLifetime; internal/web/mp_test.go: TestViewRenders; internal/web/metrics_test.go: TestViewAndStreamMetrics | Renders per view: `tickit_view_renders{kind}` and `tickit_views_watched{kind}` (the key before its first colon) and the top 20 keys on the admin statistics, keys that hold a tab id as a hash. |
| MP-13 | Edits caught by a reset or a removed page | Must | done | internal/demo/reset_test.go: TestReset; internal/wiki/rebuild_test.go: TestReset; internal/wiki/commands_test.go: TestCommits; internal/web/wiki_test.go: TestCommitAnswers; editor/test/run.mjs: 'reset: the editor stops sending, keeps its text and copies it (MP-13)' (Chromium); editor/test/run.mjs: 'readonly and gone stop sending (MP-13, MP-14.2)' (Chromium) |  |
| MP-14 | Refused while editing | Should | done | internal/web/mp_test.go: TestBlockedCommitSaysWhy; e2e/editor_mp_test.go: TestBlockedEditor (Chromium); editor/test/run.mjs: 'a refused commit is taken out and the refusal shows at the editor (MP-14)' (Chromium); editor/test/run.mjs: 'readonly and gone stop sending (MP-13, MP-14.2)' (Chromium); lith-coedit editor/test/unit/collab.test.js: 'a refused commit is taken out, later steps stay' | A blocked guest's commit answers 403 and pauses the tab's editor (`wiki.PauseEditor`): its frame stops it read-only with "Changes from this browser are paused.", above the editor and in it, and the text stays, marked "Not saved". |

## Demo operation

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| DM-01 | Visitors write without an account | Must | done | internal/access/access_test.go: TestScopes; internal/web/issue_actions_test.go: TestCreateIssue; internal/web/demo_test.go: TestStaffOnly; e2e/board_test.go: TestBoardMoveLive (Chromium); e2e/editor_coedit_test.go: TestCoEditing (Chromium) | No test cites DM-01; guests create issues, move cards and edit pages in these tests, and the login page names no shared password. |
| DM-02 | Seeded content | Must | done | internal/demo/seed/content_test.go: TestCheck; internal/demo/seed/content_test.go: TestIssueCounts; internal/demo/seed/content_test.go: TestPageCounts; internal/demo/seed/content_test.go: TestPeopleImagesTemplates; internal/demo/seed/content_test.go: TestDeterministic; internal/demo/seed_test.go: TestSeed; internal/demo/seed_test.go: TestSeedTwice; internal/demo/seed_test.go: TestPageSeeder; internal/people/people_test.go: TestSeedPeople; internal/people/people_test.go: TestSeededAsCompany |  |
| DM-03 | Periodic reset | Must | done | internal/demo/reset_test.go: TestReset; internal/demo/reset_test.go: TestResetRefusesActions; internal/web/demo_test.go: TestResetNow; internal/web/reset_test.go: TestResetShowsNoHalfSeed; internal/demo/notice_test.go: TestNotice; e2e/demo_test.go: TestResettingShows (Chromium) | While a reset runs, views render nothing and documents answer 503 with "The demo is resetting" and load again every 2 s, so no page shows the emptied or half-seeded board; every view renders once the seed is in. |
| DM-04 | Rate limits on every change | Must | done | internal/web/action_test.go: TestRateLimits; internal/demo/reset_test.go: TestCaps; internal/web/wiki_test.go: TestCommitAnswers; internal/web/images_test.go: TestUploadRateLimit; internal/images/store_test.go: TestUploadCaps; e2e/ops_test.go: TestOpsEnd (Chromium) | The classes in internal/web/action.go match the table. A limited action answers 429 alone, before its body is read (the `Tab-Id` and `Op-Id` headers name its tab and op), and its tab hears "Too many changes at once. Wait a moment." in a notice on its stream (DM-04.2), shown where the action's outcome goes. Image uploads (FR-85, a Could) are built: an upload is the class "upload" (1 every 30 s, 3 at once), counts against the 200 images of guests a day, and beyond DM-04 a person uploads 20 images or 50 MB between two resets and everyone 500 MB together (internal/images). |
| DM-05 | Content limits | Must | done | internal/tracker/decide_test.go: TestCreateIssue; internal/tracker/decide_test.go: TestCommentDeciders; internal/docnodes/rules_test.go: TestRulesRefuse; internal/demo/seed/content_test.go: TestCheck; editor/test/run.mjs: 'limits: input stops at 200 KB and at 50 table rows, and says why (DM-05)' (Chromium) |  |
| DM-06 | Safe rendering of public content | Must | done | internal/docnodes/render_test.go: TestEscapes; internal/docnodes/render_test.go: TestRenderHTML; internal/docnodes/rules_test.go: TestRulesRefuse; internal/demo/words/words_test.go: TestBlocked; internal/tracker/words_test.go: TestWordList; internal/people/words_test.go: TestRenameWordList; editor/test/run.mjs: 'link dialog (Mod-K) takes http, https and mailto only (DM-06.2)' (Chromium) |  |
| DM-07 | Moderation | Should | done | internal/tracker/moderation_test.go: TestHideIssue; internal/wiki/moderation_test.go: TestHideVersion; internal/wiki/moderation_test.go: TestHideImage; internal/web/demo_test.go: TestModerateLive; internal/web/moderation_test.go: TestModerateVersionsAndImages; internal/web/demo_test.go: TestBlockedGuest; e2e/demo_test.go: TestModerationHide (Chromium) | Issues, comments, pages, page versions and library images are moderated kinds. A hidden version leaves history and compare for everyone but staff (the newest is never hidden); a hidden image shows as "Image hidden by a moderator" in every open reader within a tick and `/img/lib/<id>` answers 404, but the editor shows it as a broken image and a browser that cached it (one day for the library, a year for an upload, privately) keeps its copy. Uploaded images (FR-85, a Could) are a kind of their own, "upload", listed newest first with a thumbnail; hiding one hides it in every page as a library image is hidden, and deleting one removes its bytes and keeps it hidden (internal/web/images_test.go: TestModerateUpload). DM-08 (a Could) is built too: internal/web/moderation_test.go: TestReports. |
| DM-09 | Search engines stay out of visitor content | Must | done | internal/web/action_test.go: TestHeaders |  |
| DM-10 | Demo notice and privacy page | Must | done | internal/demo/notice_test.go: TestNotice; e2e/prefs_test.go: TestDisplayPrefs (Chromium) | The privacy page (internal/views/pages.templ) names the operator and a contact, but no test checks its content (DM-10.2). |

## Non-functional: Security

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-01 | Access control on every request and every view | Must | done | internal/web/security_test.go: TestEveryActionRefused; internal/web/security_test.go: TestEveryActionAllowed; internal/web/security_test.go: TestEveryActionCovered; internal/web/security_read_test.go: TestEveryGetReadOnly; internal/web/security_regress_test.go: TestClosedKeysStayUnknown; internal/access/access_test.go: TestRequire; internal/web/stream_test.go: TestStreamViews; internal/web/mp_test.go: TestRemovedMemberViews; internal/web/mp_test.go: TestRoleChangeReloads | A change of membership or role through its route applies to the person's open streams at once (`rescope`): views are made again as the person may read them, and a page that lost one or shows the project loads again. Spaces have no member routes; site roles end the account's streams (`dropStaffViews`, NFR-04.2). |
| NFR-02 | One route per action, the actor from the session | Must | done | none | Enforced by Server.post (panics outside POST /cmd/), tabs.By from the session and tab_state keyed by (sid, tab_id); no test cites it. |
| NFR-03 | Cross-site requests | Must | done | internal/web/action_test.go: TestCrossSite |  |
| NFR-04 | Login sessions on the server | Must | done | internal/people/people_test.go: TestLoginLogout; internal/web/action_test.go: TestLogin | Password reset does not exist (FR-128 is Could), so its part of 04.2 does not apply. |
| NFR-05 | No default secrets | Must | done | none | The binary has no secret setting at all (opaque session ids, staff password from TICKIT_PASSWORD or stdin); no test cites it. |
| NFR-06 | GET never writes | Must | done | internal/web/action_test.go: TestGetNeverWrites; internal/web/board_test.go: TestBoardPagesNeverWrite |  |
| NFR-07 | Passwords | Must | done | internal/people/people_test.go: TestPasswords; cmd/tickit/main_test.go: TestAccountCreate |  |
| NFR-08 | Tokens | Should | done | none | The app mails or links no tokens (no registration, verification, reset or magic mail), so the criterion has nothing to apply to; no test cites it. |
| NFR-09 | Sign-in rate limits | Must | done | internal/people/people_test.go: TestLockout; internal/web/action_test.go: TestSignInLimits | Only login exists; registration and password reset are Could and not built. |
| NFR-10 | No account enumeration | Must | done | internal/people/people_test.go: TestLoginEnumeration; internal/web/action_test.go: TestLogin | Covers login, the only sign-in route that exists. |
| NFR-11 | No secrets in logs | Must | done | internal/web/webtest/secrets.go (every test's server log is searched for the passwords and session ids the test used); internal/web/action_test.go: TestLogin |  |
| NFR-12 | Client address | Must | done | none | Every address comes from edge.ClientKey, which trusts forwarding headers only from a loopback peer; no test cites it. |
| NFR-13 | Output encoding and Content Security Policy | Must | done | internal/web/design_test.go: TestDesign; internal/web/action_test.go: TestHeaders; internal/web/vendor_test.go: TestVendored | 13.1 holds by construction (expressions are Go constants in internal/views/expr.go); no test scans every page for a person's value in an expression. |
| NFR-14 | No debug or development code in production | Must | done | internal/web/action_test.go: TestHeaders | No reload, inspector or magic mail exists; -dev only changes static minification and seeding; pprof exists only in a binary built with `-tags pprof` for the load test, never in `go tool task build`'s. |
| NFR-15 | Image uploads | Should | done | internal/images/images_test.go: TestPNGStripped; internal/images/images_test.go: TestJPEGExifStripped; internal/images/images_test.go: TestRefused; internal/images/images_test.go: TestBombRefusedByHeader; internal/web/images_test.go: TestUploadIntoPage; internal/web/images_test.go: TestUploadRefusals; internal/web/images_test.go: TestUploadCheckedBeforeItsBody; internal/web/security_test.go: TestEveryActionRefused; e2e/upload_test.go: TestUploadAcrossTabs (Chromium) | FR-85 (a Could) is built (15.2): `POST /cmd/pages/{id}/image` and `/cmd/issues/{id}/description/image` take a multipart body through `tabs.Act` and the demo's guard, for a person who may edit the subject; the file is sniffed by its bytes, its header checked (5 MB, 4,096 by 4,096 pixels) before it is decoded, and it is encoded again as PNG or JPEG under a generated id (`up-` and 128 random bits), which `/img/lib/<id>` serves with that media type, `nosniff` and a sandbox CSP. A JPEG's Exif orientation is applied before the metadata goes; colour profiles go too. The right and the tab's stream are checked before the body (the `Tab-Id` and `Op-Id` headers), and the body arrives through `edge.Upload`, at least 8 KiB/s. |
| NFR-16 | Unknown ids and malformed input | Must | done | internal/web/security_fuzz_test.go: TestMalformedEveryAction; internal/web/security_fuzz_test.go: FuzzCoreActions, FuzzIssueActions, FuzzBoardActions, FuzzWikiActions, FuzzEditorActions, FuzzNotifyActions, FuzzStaffActions; internal/web/security_regress_test.go: TestCommitBeyondHead; internal/web/action_test.go: TestMalformedActions | The recorded fuzz run (3 min a target, no failure) is in docs/security.md. |
| NFR-17 | Atomic changes | Must | done | none | Every command runs in one writer savepoint with inline projectors and guard tables, and issue changes are per-field events; no test cites it. |
| NFR-18 | Uniqueness | Must | done | internal/tracker/commands_test.go: TestNumbersUnderConcurrency; cmd/tickit/main_test.go: TestAccountCreate | Constraints exist for email and username (NOCASE), scope keys, issue_keys (project, number), page_titles (lower case) and memberships. |
| NFR-19 | Security headers | Must | done | internal/web/action_test.go: TestHeaders |  |
| NFR-20 | Dependency checks | Should | done | none | .github/workflows/ci.yml runs govulncheck v1.8.0 on every push (also `go tool task vuln`); no test cites it. |

## Non-functional: Privacy

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-25 | Little personal data | Must | done | none | login_sessions keeps user agent and times without an address, and blocked addresses live in memory only (demo/guard.go); no test cites it. |
| NFR-26 | Events without personal data | Must | done | none | Events carry person ids in Meta only and people are relational with no events; no test checks events for personal fields, no test cites it. |
| NFR-27 | Erasing a person | Should | done | internal/web/demo_test.go: TestEraseAccount; internal/demo/reset_test.go: TestReset | "Erase" on `/admin` (`POST /cmd/admin/erase`, `people.EraseAccount`) deletes the account's rows of people and accounts, its login sessions, memberships, notifications and saved filters; its events keep the id alone and it shows as "Deleted person", mentions too; the security log records "Account erased". A mention's label in the stored documents and the page search text still hold the name it was made with. |
| NFR-28 | Log retention | Must | partly | internal/web/logs_test.go: TestLogsHoldNoContent | A full run with marker text in every issue, page and comment leaves none in the app's log (request paths name no page slug or label). On libretto Caddy keeps the access log 720 h and Loki 720 h with retention on (checked 2026-10-20); the journal is capped by size (1 GB, `SystemMaxUse`), not by 30 days. |
| NFR-29 | No third parties | Must | done | internal/web/vendor_test.go: TestVendored; internal/web/design_test.go: TestDesign |  |
| NFR-30 | Pseudonyms in presence and activity | Must | done | internal/people/people_test.go: TestGuestIdentity | Guests show as pseudonym and face derived from a hash of the session id, accounts by display name; no test cites NFR-30 itself. |
| NFR-31 | One cookie and a privacy page | Must | partly | internal/web/prefs_test.go: TestPrefs; internal/web/action_test.go: TestHeaders; e2e/prefs_test.go: TestDisplayPrefs (Chromium) | One cookie and the privacy page are in place; the legal check of the wording against FADP and GDPR (Q25) is not recorded. |

## Non-functional: Performance and realtime

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-35 | From commit to screen | Must | partly | cmd/tickit-load (go tool task load); cmd/tickit/main.go: tickit_tick_seconds | On lith v0.4.1 with the feeds' topics and reads, the home page's cache and the issue page's indexes: commit to frame, measured to the client as an upper bound, p50 43 ms (commits), 49 ms (moves) and 51 ms (fields), p95 76 to 84 ms; ticks overrun 0.54 %, not under 0.1 % (benchmarks/RESULTS.md, Capacity on lith v0.4.1). |
| NFR-36 | Action answer time | Must | done | cmd/tickit-load (go tool task load) | p95 3.6 to 5.5 ms at the load of NFR-38. |
| NFR-37 | Frame sizes and slow links | Should | partly | internal/web/board_test.go: TestBoardFrameSize; internal/web/stream_test.go: TestSlowLink; cmd/tickit-load (go tool task load) | 25 list rows 36 KB and slow links at most 4 frames a second pass; a board of 100 cards with points, a label and a face on every card is 64.6 KB, over 60 KB. The test board of 100 cards is 59,866 bytes, and in swimlanes 59,070 (by epic) and 56,854 (by assignee): internal/web/workflow_test.go: TestBoardSwimlanes. A window of a feed, 50 entries, is under 60 KB: internal/web/feed_test.go: TestFeedWindows. |
| NFR-38 | Capacity | Must | partly | internal/web/board_load_test.go: TestBoardLoad; internal/web/board_test.go: TestBoardOneRenderPerTick; cmd/tickit-load (go tool task load) | 200 streams with 10,000 issues, 2,000 pages and 1,000,000 events, every frame delivered and NFR-36 met; NFR-35 is missed narrowly. |
| NFR-39 | Page load | Must | partly | cmd/tickit-load (go tool task load) | Documents p95 26 to 90 ms under the load; a stream's first frames come with the next tick and its render, p95 72 ms from the request, not within 50 ms. |
| NFR-40 | Search speed | Should | done | internal/web/perf_test.go: TestSearchSpeed | 10,000 issues and 2,000 pages, p95 under 200 ms, recorded in benchmarks/RESULTS.md; pages go into the index through its hook, not the wiki's commands. |
| NFR-41 | Bounded resources | Must | done | internal/tabs/tabs_test.go: TestSweep; internal/demo/reset_test.go: TestReset; cmd/tickit-load (go tool task load) | The 201st stream answers 503, and an address holds 40 streams (the hub's per-client cap, 429 beyond); anonymous memory at most 530 MiB under MemoryMax 640M, though above MemoryHigh 512M, where systemd throttles. |
| NFR-42 | Reconnect after a deploy | Must | done | internal/web/stream_test.go: TestReconnect; internal/web/stream_test.go: TestRefreshAfterDeploy; e2e/editor_reconnect_test.go: TestEditorReconnect (Chromium); e2e/ops_test.go: TestOpsEnd (Chromium); cmd/tickit-load (go tool task load) | All 200 streams back with current frames 1.0 s after /healthz. While the server stops, pages say "Updating, back in a moment" (`live.Options.Stopping`); a page a former build rendered offers a reload (`live.Spec.Refresh`), with its views current. |
| NFR-43 | Growth of the event log from co-editing | Must | partly | internal/wiki/bench_test.go: TestFoldTime; internal/wiki/commands_test.go: TestFold; internal/wiki/commands_test.go: TestCommits; internal/tracker/description_test.go: TestDescriptionFold; cmd/tickit-load (go tool task load) (-sustain; tickit-load fold) | The fold keeps up and co-editing adds about 76 MB a day; on lith v0.4.2 a fold command holds the writer p50 0.7 to 2.6 ms at 10 to 100 events (tickit-load fold on a sustained run's database), under 5. But the issue changes of NFR-38 add 620 MB a day. |
| NFR-44 | Search stays right through rebuilds | Must | done | internal/search/search_test.go: TestRebuild |  |

## Non-functional: Accessibility

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-45 | WCAG 2.2 AA | Should | partly | e2e/a11y_test.go: TestAxe (Chromium, TICKIT_AXE); internal/web/contrast_test.go: TestContrast | axe finds no serious or critical issue on the pages of NFR-45 in both themes and a keyboard walk is recorded (docs/accessibility.md); no screen reader check was run (NFR-45.1). |
| NFR-46 | Zoom | Must | done | e2e/a11y_test.go: TestNarrowPages (Chromium); e2e/dashboard_layout_test.go: TestDashboardRowsFit (Chromium, Firefox); internal/web/stream_test.go: TestZoom; internal/web/prefs_test.go: TestEveryPage | Eight pages at 320 CSS pixels with no sideways scroll; zoom is allowed on every page. |
| NFR-47 | Keyboard | Must | done | e2e/board_test.go: TestBoardKeyboardMove (Chromium); e2e/list_test.go: TestListTypingAndHistory (Chromium); e2e/list_test.go: TestListPanel (Chromium); e2e/notify_test.go: TestBellLive (Chromium); e2e/sbnav_test.go: TestPageTree, TestMoveDialogTree, TestAdminRowMenu (Chromium) | The page tree's keys, the Move dialog's tree and the admin's row menu (Escape gives the focus back to the row's button) have tests; dialogs and panels have none for the focus return. |
| NFR-48 | Live updates and assistive technology | Must | done | e2e/notify_test.go: TestBellLive (Chromium); e2e/issue_test.go: TestIssueEditInPlace (Chromium); e2e/frames_test.go: TestFramesKeepPlace (Chromium) |  |
| NFR-49 | Accessible editor | Should | done | editor/test/run.mjs: "toolbar: named buttons, aria-pressed, one tab stop, Alt+F10 and arrows (NFR-49, NFR-47)" (Chromium); editor/test/run.mjs: "remote carets: bar and name in the person's hue, at their place, hidden from screen readers (MP-06)" (Chromium); editor/test/run.mjs: "toDOM writes docs/editor.md's HTML of a document" (Chromium) | Node tests with puppeteer-core (npm test in editor/), not go test. |
| NFR-50 | Semantics, colour and contrast | Must | done | internal/web/contrast_test.go: TestContrast; internal/ui/ui_test.go: TestIcons |  |
| NFR-51 | Language and dates | Must | done | internal/ui/ui_test.go: TestAgo; internal/web/design_test.go: TestDesign; internal/web/prefs_test.go: TestEveryPage |  |

## Non-functional: Browsers

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-55 | Supported browsers | Must | partly | internal/web/action_test.go: TestCrossSite; e2e tests (Chromium; Firefox 158.0.1 with LITH_BROWSER=firefox) | The whole e2e suite passes in Chromium and in Firefox 158.0.1 (2026-10-20, docs/accessibility.md); the touch tests skip in Firefox, whose WebDriver BiDi has no touch emulation. Nothing runs Safari, iOS, Android or Edge, and the brotli fallback for old Safari has no test here. |
| NFR-56 | Small screens and touch | Should | done | e2e/touch_test.go: TestBoardTouchDrag (Chromium); e2e/touch_test.go: TestEditorTouch (Chromium); e2e/a11y_test.go: TestNarrowPages (Chromium); e2e/board_test.go: TestBoardKeyboardMove (Chromium); e2e/board_shots_test.go: TestBoardShots (Chromium) | Touch is Chromium's emulation over CDP, not a real phone: lanes of at least 260 pixels, a card dragged by its grip, and the editor taking typing, a list and a link by taps. |

## Non-functional: Deployment on the shared host

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-60 | Build | Must | done | deploy/smoke-test.sh (go tool task smoke) | No Go test cites it; hugo stays in go.mod only as an indirect dependency of the air dev tool, not of the binary. |
| NFR-61 | systemd unit | Must | done | none | No test cites it; deploy/tickit.service has the user, StateDirectory, 127.0.0.1:7105, loopback-only network, MemoryMax and TimeoutStopSec=30. |
| NFR-62 | Caddy block | Must | done | none | No test cites it; deploy/tickit.caddy only proxies, without encode, replaces X-Forwarded-For and keeps the log at 0640 for 720 h. |
| NFR-63 | Drop-ins for monitoring | Must | done | none | No test cites it; deploy/observability holds the three files and deploy/setup-host.sh installs them with Alloy rollback and no restart. |
| NFR-64 | Deploy and rollback | Must | done | none | No test cites it; deploy/tickit-deploy waits 30 s for "ok <version>", restores the old binary, and has a rollback mode. |
| NFR-65 | Migrations | Must | done | cmd/tickit/migrations_test.go: TestPending; cmd/tickit/migrations_test.go: TestMigrationsPendingCommand |  |
| NFR-66 | Shutdown | Must | done | internal/web/stream_test.go: TestReconnect; deploy/smoke-test.sh (exit on SIGTERM) | The shutdown order in cmd/tickit/main.go has no Go test of its own. |
| NFR-67 | Configuration | Must | done | none | No test cites it; flags read TICKIT_* from the environment, deploy/tickit.env.example and docs/deploy.md list every setting with its default. |

## Non-functional: Backups

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-70 | Daily copies | Should | done | cmd/tickit/migrations_test.go: TestBackupChecksCopy | The timer, VACUUM INTO, 30 copies and the integrity check are in place; the restore of a daily copy was tried on libretto on 2026-10-20 (docs/deploy.md, "A restore, once"). |
| NFR-71 | A copy before a migrating deploy | Must | done | cmd/tickit/migrations_test.go: TestPending | deploy/tickit-deploy takes the copy; the script itself has no test. |
| NFR-72 | Recovery by reseed | Must | done | internal/demo/reset_test.go: TestReset | The drill of 2026-10-20 restored the newest daily copy into a scratch database on libretto and served it with the release binary: the staff account was there; the moderation log was empty, since nothing had been moderated yet. TestReset proves the reseed. |

## Non-functional: Observability

| ID | Title | Priority | Status | Tests | Note |
| --- | --- | --- | --- | --- | --- |
| NFR-75 | Health and readiness | Must | done | deploy/smoke-test.sh (go tool task smoke) | No Go test cites it; the 503 naming the failing check comes from lith's edge.Ready and is not tested here. |
| NFR-76 | Metrics | Must | done | cmd/tickit/main_test.go: TestLoopback; internal/web/metrics_test.go: TestViewAndStreamMetrics; deploy/smoke-test.sh | docs/architecture.md, "Metrics", maps the grouping document's metrics onto the exported names (76.2); group sizes (watchers per view) are not exported. |
| NFR-77 | Logs | Must | done | internal/web/action_test.go: TestLogin | slog text handler at Info by default and edge.LogRequests; TestLogin covers NFR-11, nothing else tests the log lines. |
| NFR-78 | Alerts and dashboard | Should | done | none | No test cites it; deploy/observability/rules-tickit.yaml is hand-written. On libretto on 2026-10-20 Grafana's rules API showed its three rules and the host's rules for probes, tick overruns, parked projectors and failed units evaluating with health ok; none has fired yet. |

## Gaps

The Must requirements that are partly done, most important first. No Must requirement is entirely not done.

- NFR-43 Growth of the event log: 24 hours of NFR-38's issue changes alone exceed 500 MB (NFR-43.2).
- NFR-35 From commit to screen: at NFR-38's load the median is a few milliseconds over 50 ms and 0.38 % of ticks overrun (asks under 0.1 %).
- NFR-39 Page load: a stream's first frames arrive with the next tick plus its render, up to 74 ms after the request.
- NFR-38 Capacity: met for streams, data and actions, but not with NFR-35 as it asks.
- NFR-55 Supported browsers: the browser tests pass in Chromium and Firefox 158; nothing runs Safari, iOS, Android or Edge.
- NFR-28 Log retention: the app's logs hold no content (tested), and Caddy and Loki keep 30 days, but the journal on libretto is capped by size, not by 30 days.
- NFR-31 One cookie and a privacy page: the legal check of the wording (Q25) is not recorded.
