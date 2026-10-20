---
title: Decision log
parent: Architecture
position: 2
author: s-tomasz
created: -28d 16:00
labels: [architecture, decisions]
---
Decisions that shape the code. Each one gets a number that is never reused. Use the decision log template for new ones.

| No. | Decision | Status | Date |
| --- | --- | --- | --- |
| 1 | PostgreSQL for everything, including the job queue | Accepted | two years ago |
| 2 | The shop is rendered on the server | Accepted | two years ago |
| 3 | One TypeScript codebase for the app on both platforms | Accepted | last year |
| 4 | Feature flags live in the database, not in config files | Accepted | last year |
| 5 | Money in rappen, as integers | Accepted | last year |
| 6 | No deploys on Friday afternoon | Accepted | last year |
| 7 | Keep summit one service, with cairn as the exception | Accepted | this month |

The only one with its own page so far is [[page:ADR 7: Keep summit one service]]. The others are older than this handbook. If you know their story, write it down.
