---
title: Pack lists
parent: Roadmap
position: 2
author: s-priya
created: -28d 14:00
labels: [requirements, app, pack-lists]
edited: -12d 17:00
editor: s-amara
message: Screens and the beta numbers
versions:
  - at: -28d 14:00
    by: s-priya
    message: First draft
    body: |
      ## Problem

      People forget things. We sell things.

      ## Idea

      A packing list in the app, from a template.
  - at: -24d 11:00
    by: s-priya
    message: Requirements after the interviews
    body: |
      ## Problem

      9 of 12 hikers we interviewed pack from a list they keep somewhere: a note, a spreadsheet, a photo. Three forgot the same thing twice.

      ## Requirements

      | No. | Requirement | Must or should |
      | --- | --- | --- |
      | 1 | Start a list from a trip template | must |
      | 2 | Tick off items, see the total weight | must |
      | 3 | Suggest what is missing, add it to the cart | must |
      | 4 | Share a list | should |
      | 5 | Works without signal | should |

      Research: [[page:Pack lists: research notes]]
---
Epic: [[APP-1]]. Designs by @s-amara, built by @s-yuki, API by @s-mateo.

## Problem

9 of 12 hikers we interviewed pack from a list they keep somewhere: a note on the phone, a spreadsheet, a photo of last year's paper list. Three forgot the same thing twice. Nobody's list knows what their gear weighs.

## Requirements

| No. | Requirement | Must or should | Issue |
| --- | --- | --- | --- |
| 1 | Start a list from a trip template | must | [[APP-6]] |
| 2 | Tick off items, see the total weight | must | [[APP-7]] |
| 3 | Suggest what is missing, add it to the cart | must | [[APP-9]] |
| 4 | Share a list with a link | should | [[APP-12]] |
| 5 | Works without signal | should | [[APP-4]] |

## Screens

![A pack list in the app](lib:app-pack-list)

Ticked items stay on the list, faded. People like to see what they already have, and the weight needs them.

## Measures

- Share of app users with at least one list after 30 days: target 25 %
- Revenue from "Add missing items" per list: no target yet, we watch it
- Beta, two weeks: 214 lists, the hut trip template most used

## Not in 3.0

Sharing, own templates, and lists per person. All three came up in the beta ([[APP-25]]).
