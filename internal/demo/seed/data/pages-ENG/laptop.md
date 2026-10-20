---
title: Set up your laptop
parent: Your first week
position: 1
author: s-mateo
created: -28d 11:00
labels: [onboarding, howto]
---
Everything runs on one laptop: summit, cairn, porter and a PostgreSQL in a container.

## Tools

```bash
# Go and the internal CLI
curl -fsSL https://tools.example.com/install.sh | sh
alp version

# Clone and build
git clone https://git.example.com/alpenglow/summit.git
cd summit
go build ./...
```

## A database with test data

```bash
alp dev db up            # PostgreSQL 18 in a container, port 5433
alp dev db seed --small  # 2,000 products, 500 orders, all made up
alp dev run              # summit on :8080, cairn on :8081, porter in the background
```

Open http://localhost:8080 and buy a backpack. Nobody will ship it.

## Access

You need three accounts on day one. @s-tomasz creates them:

1. The repository and the deploy pipeline
2. The back office, with the staff role
3. The on-call tool, as a shadow

> If a step on this page is wrong, fix the page while you remember what went wrong.
