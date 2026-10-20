---
title: Incident response
parent: Incidents
position: 1
author: s-tomasz
created: -29d 11:00
labels: [incident, on-call]
---
![Incident response](lib:incident-response)

## Roles

- **Lead**: decides, keeps the timeline, posts updates. Not the person typing the fix.
- **Fixer**: one or two people, working, not talking to customers.
- **Voice**: @s-lena or @s-priya, for the status page and support answers.

## Updates

Every 30 minutes, even if nothing changed. "Still looking, next update at 07:30" is an update.

## Severity

| Level | Example | Who wakes up |
| --- | --- | --- |
| 1 | Nobody can pay | on-call and the lead |
| 2 | Part of the shop is wrong, like [[OPS-21]] | on-call |
| 3 | Something internal is broken | nobody, it waits for the morning |
