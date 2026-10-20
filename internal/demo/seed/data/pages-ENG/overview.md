---
title: Architecture overview
parent: Architecture
position: 1
author: s-tomasz
created: -29d 14:00
labels: [architecture]
---
![Architecture overview](lib:architecture-overview)

## The pieces

- **summit** is the Go service behind the shop and the app. It renders the shop's pages, serves the app's API, and owns the database.
- **cairn** is the search service. It reads product changes from the outbox and answers searches and suggestions. See [[page:Search on cairn]].
- **porter** runs jobs: order emails, the stock sync with the warehouse, image sizes.
- **PostgreSQL** holds everything that matters. Backups every night, tested every month: [[page:Restore a database backup]].

Partners: Paygrove takes payments and calls us back with webhooks ([[page:Checkout flow]]); the warehouse drops a stock file every 15 minutes and picks up shipments.

## The data

![Core data model](lib:data-model)

Money is stored in rappen, as integers. If you see a float near a price, that is a bug, and probably [[WEB-44]].

## Boring on purpose

One service, one database, one queue table. We are six people. Every extra moving part needs someone awake at 03:00 to look after it. The exceptions are written down in the [[page:Decision log]].
