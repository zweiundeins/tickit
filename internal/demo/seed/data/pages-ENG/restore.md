---
title: Restore a database backup
parent: Runbooks
position: 2
author: s-tomasz
created: -29d 16:00
labels: [runbook, database, backup]
---
Last tried 29 days ago on a scratch server ([[OPS-4]]): 18 minutes for 41 GB.

## 1. Find the backup

```bash
alp db backups list --env prod | head -5
```

## 2. Restore it somewhere new

Never restore over production. Restore next to it, check, then switch.

```bash
alp db restore prod-nightly-latest --to restore-check
```

## 3. Check it

```sql
SELECT count(*) AS orders, max(placed_at) AS newest FROM orders;
SELECT count(*) AS products FROM products WHERE archived_at IS NULL;
```

The newest order should be from last night, before 03:00. About 41,000 products.

## 4. Switch

Only with @s-tomasz or whoever leads the incident:

```bash
alp db promote restore-check --env prod --confirm
```

- [ ] Shop in read-only mode first
- [ ] Note the time of the backup in the incident channel: orders after it are lost and must be recovered from Paygrove
