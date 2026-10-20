---
title: On-call
parent: Ways of working
position: 3
author: s-tomasz
created: -24d 10:00
labels: [process, on-call]
---
One week each, Monday to Monday, @s-tomasz and @s-mateo for now, @s-yuki from next month. New people shadow for a month first.

![Operations dashboard](lib:ops-dashboard)

## What pages at night

Only what customers feel. Everything else waits for the morning.

```yaml
routes:
  - match: { severity: page }
    notify: on-call-phone
    hours: always
  - match: { severity: ticket }
    notify: ops-board
    hours: "08:00-18:00"
  - match: { env: staging }
    notify: ops-board
    hours: "08:00-18:00"
```

| Alert | Pages at night | Runbook |
| --- | --- | --- |
| CheckoutErrorRateHigh | yes | [[page:Incident response]] |
| StockFileStale | yes | [[page:Stock sync failures]] |
| StockSyncGuardFailed | no, the shop keeps the last good stock | [[page:Stock sync failures]] |
| CertificateExpiresSoon | no | renew in the morning |

Paging rules are being updated in [[OPS-41]].

## Service level objectives

Drafts in [[OPS-19]]: checkout 99.9 % without errors, search 99 % within 300 ms, emails 99 % within 5 minutes.

## After a night with a page

Sleep in. Write two lines in the channel about what happened, and file an issue if the alert was not worth waking up for.
