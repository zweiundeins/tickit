---
title: ADR 7: Keep summit one service
parent: Decision log
position: 1
author: s-tomasz
created: -27d 11:00
labels: [decisions, architecture, search]
---
## Status

Accepted. @s-tomasz, @s-mateo and @s-priya.

## Context

Search is slow and bad at typos ([[WEB-19]]). Fixing it inside summit means search indexes in the main database, more load on it during the sale, and slow migrations. Every few months someone proposes splitting summit into services. We are six people.

## Decision

summit stays one service. Search becomes the one exception: cairn, a separate service with its own index, fed from the outbox ([[OPS-2]]).

## Consequences

- One more thing to deploy, watch and page on. The runbooks cover it.
- A search outage does not take the checkout down, and the other way round.
- The next proposal to split summit starts by reading this page.

## Options we did not take

1. Search inside PostgreSQL with full text indexes: good enough for now, not for typos and ranking.
2. A hosted search product: costs more than the server, and our catalog would leave the country.
3. Split summit into catalog, orders and accounts: three times the deploys, no customer benefit.
