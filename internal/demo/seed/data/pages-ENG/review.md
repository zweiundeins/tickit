---
title: Code review guide
parent: Ways of working
position: 1
author: s-tomasz
created: -26d 14:00
labels: [process, review]
---
## For the author

- Small changes. Under 400 lines, or tell the reviewer why not.
- Say what changed and why in one or two sentences. Link the issue, like [[WEB-10]].
- Review your own diff first. You will find the stray debug line.

## For the reviewer

- Answer within one working day
- Say what kind of comment it is: **must** (blocks the merge), **should**, or **nit**
- Ask questions instead of giving orders. "What happens when the webhook arrives twice?" teaches more than "add a check".

## Go, the house style

```go
// Good: the error says what failed and with which input.
if err != nil {
	return fmt.Errorf("load stock file %s: %w", name, err)
}

// Not so good: the reader of the log learns nothing.
if err != nil {
	return err
}
```

- No panics outside `main`
- Every handler takes a `context.Context` and passes it on
- Tests next to the code, table-driven when there are more than two cases

> A review is a conversation between colleagues, not an exam.
