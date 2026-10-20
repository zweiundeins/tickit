---
title: Stock sync failures
parent: Runbooks
position: 3
author: s-mateo
created: -17d 10:00
labels: [runbook, warehouse]
---
The warehouse drops `stock.csv` on our SFTP server every 15 minutes, and porter loads it. Since [[OPS-24]] a guard refuses a file that looks wrong.

![Stock sync with its guard](lib:stock-sync-flow)

## The guard refused a file

Nothing is broken for customers: the shop keeps the last good stock. Look at why:

```bash
alp porter logs stock-sync --since 1h | grep refused
```

| Message | Meaning | Do |
| --- | --- | --- |
| has 1184 rows, yesterday's had 41022 | cut-off file | wait for the next file |
| would sell out 9 % of products | broken file, or a real problem | call the warehouse |
| no new file for 30 minutes | their export stopped | call the warehouse |

## Load a file by hand

Only a file you have looked at:

```bash
alp porter run stock-sync --file /srv/sftp/warehouse/stock-0530.csv
```

Careful: this sends back-in-stock events again ([[OPS-43]]).

## Check what the shop shows

```sql
SELECT count(*) FROM stock_levels WHERE on_hand = 0;
-- usually between 300 and 600
```

The guard itself, for the curious:

```go
if soldOut := newlySoldOut(prev, next); soldOut*100 > len(next)*5 {
	return fmt.Errorf("would sell out %d %% of products: refusing", soldOut*100/len(next))
}
```
