---
title: Release checklist
parent: Ways of working
position: 2
author: s-lena
created: -25d 15:00
labels: [process, release, checklist]
edited: -6d 11:30
editor: s-yuki
message: App releases and the store review
versions:
  - at: -25d 15:00
    by: s-lena
    message: The list I use anyway
    body: |
      - [ ] Tests green
      - [ ] Tried on staging
      - [ ] Deployed
  - at: -20d 09:30
    by: s-tomasz
    message: Sale rule and the database
    body: |
      ## Shop and platform

      - [ ] Tests green on main
      - [ ] Tried on staging, with the smoke tests
      - [ ] Migrations work with the previous version of summit
      - [ ] Not Friday after 15:00, not in the first hour of a sale
      - [ ] Deployed with the canary, see [[page:Deploy to production]]
---
Copy this list into the issue or the merge request and tick it off there.

## Shop and platform

- [ ] Tests green on main
- [ ] Tried on staging, with the smoke tests
- [ ] Migrations work with the previous version of summit
- [ ] Feature flag for anything customers see, default off
- [ ] Not Friday after 15:00, not in the first hour of a sale
- [ ] Deployed with the canary, see [[page:Deploy to production]]
- [ ] Watched the dashboard for 15 minutes

## App

- [ ] Branch cut on Monday of the second week
- [ ] Beta for at least three days, crash-free sessions at or above 99.5 %
- [ ] Store texts and screenshots updated ([[APP-35]])
- [ ] Account deletion reachable from the profile, the stores check it ([[APP-37]])
- [ ] Staged rollout: 10 %, 50 %, 100 %, a day each ([[APP-40]])

## After

- [ ] Release notes in the product sync
- [ ] Issues moved to Done, with what was shipped in the last comment
