---
title: Product sync: store review
parent: Meeting notes
position: 2
author: s-priya
created: -5d 10:00
labels: [meetings, app]
---
Attendees: @s-priya, @s-tomasz, @s-yuki, @s-amara, @s-lena. @s-mateo was fixing [[WEB-44]].

## Notes

- Store review rejected 3.0: account deletion must start inside the app ([[APP-37]]). Fair point, honestly.
- @s-yuki builds the profile entry ([[APP-38]]), @s-mateo makes the endpoint proper ([[OPS-39]]).
- One-page checkout at 3.6 % conversion. Nobody wants to turn it off.
- Order tracking shows Delivered too early ([[APP-28]]). Three more emails this week.

## Decisions

- Resubmit as soon as [[APP-38]] passes review; no other changes in that build.
- PostgreSQL upgrade only if the window is confirmed by Friday, otherwise after the sale.

## Action items

- [ ] @s-yuki resubmits 3.0
- [ ] @s-priya writes the help page on account deletion ([[APP-39]])
- [ ] @s-lena tests the deletion flow with a real test account, including an open order
- [ ] @s-tomasz decides on the database window on Friday
