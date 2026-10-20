---
title: Postmortem: the sold-out socks
parent: Incidents
position: 2
author: s-tomasz
created: -18d 10:30
labels: [incident, postmortem, warehouse]
edited: -15d 16:00
editor: s-tomasz
message: Follow-ups updated, the first guard is live
versions:
  - at: -18d 10:30
    by: s-tomasz
    message: Notes from the meeting
    body: |
      ## Summary

      From 06:00 to 09:02, 412 products showed as sold out. Cause: a cut-off stock file.

      ## Timeline

      - 05:45 the warehouse export starts writing stock.csv
      - 05:46 porter loads the half-written file
      - 06:25 incident declared
      - 09:02 stock restored by hand

      ## Follow-ups

      - Refuse short files
      - Ask the warehouse for a checksum
  - at: -17d 09:15
    by: s-mateo
    message: Technical detail and the latency chart
    body: |
      ## Summary

      From 06:00 to 09:02, 412 products showed as sold out, most of them socks and base layers. Cause: porter loaded a stock file the warehouse was still writing, and took every missing row as zero in stock.

      ## Timeline

      - 05:45 the warehouse export starts writing stock.csv
      - 05:46 porter picks up the half-written file, 1,184 of 41,000 rows
      - 06:10 first customer email to support
      - 06:25 incident declared, @s-tomasz leads ([[OPS-21]])
      - 06:50 cause found by @s-mateo
      - 09:02 stock restored from the 05:30 file

      ![API latency, last 30 days](lib:chart-api-latency)

      ## Follow-ups

      - Refuse short files
      - Refuse files that sell out many products at once
      - Ask the warehouse for a checksum
---
## Summary

From 06:00 to 09:02, 412 products showed as sold out, most of them socks and base layers. Cause: porter loaded a stock file the warehouse was still writing, and took every missing row as zero in stock. About 23 customers wrote to us; we estimate 60 lost orders.

## Timeline

- 05:45 the warehouse export starts writing stock.csv
- 05:46 porter picks up the half-written file, 1,184 of 41,000 rows
- 06:10 first customer email to support
- 06:25 incident declared, @s-tomasz leads ([[OPS-21]], [[WEB-38]])
- 06:50 cause found by @s-mateo
- 09:02 stock restored from the 05:30 file, @s-lena answers every customer

![API latency, last 30 days](lib:chart-api-latency)

The latency spike is the shop retrying carts full of suddenly sold-out socks.

## What went well

- Support noticed before the alerts did, and said so in the channel at once
- The 05:30 file was still there, so the restore was simple

## What went badly

- Nothing checked the file before loading it
- We had no alert for "a lot of products sold out at once"
- The runbook for the stock sync did not exist. It does now: [[page:Stock sync failures]]

## Follow-ups

| What | Issue | Status |
| --- | --- | --- |
| Refuse short files | [[OPS-24]] | done |
| Refuse files that sell out more than 5 % | [[OPS-25]] | in review |
| Alert on a stale stock file | [[OPS-26]] | in progress |
| Checksum line from the warehouse | [[OPS-27]] | waiting for them |
