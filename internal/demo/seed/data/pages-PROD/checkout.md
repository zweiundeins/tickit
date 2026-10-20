---
title: One-page checkout
parent: Roadmap
position: 3
author: s-priya
created: -27d 10:00
labels: [requirements, checkout]
---
Epic: [[WEB-1]].

## Problem

A third of the people who start our checkout leave it. Most leave on the delivery step, where we ask for the address a second time.

## What we build

![The one-page checkout](lib:checkout-one-page)

| Requirement | Issue | State |
| --- | --- | --- |
| Address, delivery and payment on one page | [[WEB-6]] | in review |
| Summary beside it, vouchers without a reload | [[WEB-7]] | in progress |
| Remember a returning customer's address | [[WEB-9]] | to do |
| 50 % of sessions behind a flag | [[WEB-8]] | done |

## Results so far

![Checkout conversion by week](lib:chart-checkout-conversion)

At 50 % of sessions for three weeks: 3.6 % conversion against 2.8 %. Voucher tickets went from eleven a month to two a week.

## Voucher messages

A voucher that does not apply says why, with the amount that is missing. summit answers:

```json
{
  "code": "FIRSTSNOW",
  "applies": false,
  "reason": "minimum",
  "missingChf": "12.00",
  "message": "Add CHF 12.00 more for this voucher"
}
```

## Open questions

- Do we keep the guest checkout as the default? Yes, for now. Accounts are optional and stay optional.
- When do we remove the old checkout? After the sale ([[WEB-35]]).
