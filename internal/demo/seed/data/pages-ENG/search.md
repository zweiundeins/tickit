---
title: Search on cairn
parent: Architecture overview
position: 2
author: s-mateo
created: -24d 15:00
labels: [architecture, search]
---
![Search indexing on cairn](lib:search-indexing)

cairn keeps its own index, built from the outbox. A product change reaches search in under five seconds. Every night at 03:10 a full rebuild writes a new index and moves the alias when it is done ([[OPS-7]]).

## Where we are

- 10 % of searches go to cairn since [[OPS-13]]
- Everyone moves over with [[OPS-34]], after the synonyms bug ([[OPS-18]]) and the ranking of titles ([[WEB-19]])

## Ranking

| Field | Weight |
| --- | --- |
| Title | 3 |
| Category | 2 |
| Description | 1 |

Typos up to one letter in words of five letters or more. "Backpak" finds backpacks, "sock" does not find "rock".
