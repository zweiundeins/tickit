---
title: Roadmap
position: 2
author: s-priya
created: -29d 09:00
labels: [roadmap, planning]
edited: -5d 16:30
editor: s-priya
message: Store review delay, PostgreSQL may move after the sale
versions:
  - at: -29d 09:00
    by: s-priya
    message: Autumn plan
    body: |
      | Now | Next | Later |
      | --- | --- | --- |
      | One-page checkout | Offline pack lists | Boot fit finder |
      | Pack lists | Back-in-stock alerts | Rental gear |
      | First Snow Sale | Search for everyone | Trail journal 2.0 |
  - at: -18d 14:00
    by: s-tomasz
    message: Platform work after the incident
    body: |
      ![Roadmap board](lib:roadmap-board)

      | Now | Next | Later |
      | --- | --- | --- |
      | One-page checkout | Offline pack lists | Boot fit finder |
      | Pack lists | Back-in-stock alerts | Rental gear |
      | First Snow Sale | Search on cairn for everyone | Trail journal 2.0 |
      | PostgreSQL 18 | | |
      | Stock sync hardening | | |
---
![Roadmap board](lib:roadmap-board)

Now means this cycle, next means the one after, later means we believe in it but have not planned it.

| Now | Next | Later |
| --- | --- | --- |
| One-page checkout ([[WEB-1]]) | Offline pack lists ([[APP-4]]) | Boot fit finder |
| Pack lists ([[APP-1]]) | Back-in-stock alerts ([[APP-3]]) | Rental gear |
| First Snow Sale ([[WEB-2]]) | Search on cairn for everyone ([[OPS-34]]) | Trail journal 2.0 |
| PostgreSQL 18 ([[OPS-1]]), may move after the sale | Fewer returns for size ([[WEB-5]]) | |
| Stock sync hardening ([[OPS-23]]) | | |

## Changes since the last version

- App 3.0 slips by about four days: the store review asked for account deletion in the app ([[APP-37]]). If it is not at 100 % two days before the sale, the sale runs on 2.8.
- The PostgreSQL upgrade waits for a maintenance window ([[OPS-11]]). We will not do it in the sale week.

## How we choose

1. Does it help people get the right gear the first time?
2. Can we measure that it did?
3. Is it the smallest thing that would tell us?
