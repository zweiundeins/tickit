---
title: Checkout flow
parent: Architecture overview
position: 1
author: s-mateo
created: -26d 10:00
labels: [architecture, checkout]
---
![Checkout sequence](lib:checkout-sequence)

## The rule that matters

An order is paid when Paygrove's webhook says so, never when the customer comes back to the shop. Some customers close the tab after paying; their orders must still ship.

```go
// The webhook is the only place that marks an order paid.
func (h *Webhooks) PaymentPaid(ctx context.Context, ev paygrove.Event) error {
	return h.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := orders.MarkPaid(tx, ev.OrderID, ev.Amount); err != nil {
			return err
		}
		return outbox.Add(tx, "order.paid", ev.OrderID)
	})
}
```

Webhooks can arrive twice, or before the customer is back. `MarkPaid` does nothing for an order that is already paid.

## Stock

Stock is reserved when the order opens and released after 30 minutes without payment. The one-page checkout ([[WEB-1]]) does not change this.
