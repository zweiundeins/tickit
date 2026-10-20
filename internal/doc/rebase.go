package doc

// Rebase maps steps made on the document before `since` over `since`,
// applies to doc what still applies, and returns the new document and the
// steps as applied. It is the writer's rebase: applyCommit of
// @stepwisehq/prosemirror-collab-commit. A mapped step that no longer fits
// is dropped, as the browser drops it; any other error refuses the commit.
func Rebase(doc Doc, since, steps []Step) (Doc, []Step, error) {
	newMaps := make([]*StepMap, len(since))
	for i, s := range since {
		newMaps[i] = s.StepMap()
	}
	own := make([]*StepMap, len(steps))
	for i, s := range steps {
		own[i] = s.StepMap()
	}
	mapping := NewMapping(own).Invert()
	mapping.AppendMapping(NewMapping(newMaps))
	applied := make([]Step, 0, len(steps))
	mapFrom := len(steps)
	for i := range steps {
		mapped := steps[i].Map(mapping.Slice(mapFrom, len(mapping.maps)))
		mapFrom--
		if mapped == nil {
			continue
		}
		next, err := mapped.Apply(doc)
		if err != nil {
			if IsStepFailed(err) {
				continue
			}
			return nil, nil, err
		}
		doc = next
		applied = append(applied, mapped)
		mapping.AppendMap(mapped.StepMap(), -1)
		mapping.SetMirror(mapFrom, len(mapping.maps)-1)
	}
	return doc, applied, nil
}

// Apply applies steps in order, as Transform.step does: the first step
// that fails or throws stops it.
func Apply(doc Doc, steps []Step) (Doc, error) {
	for _, s := range steps {
		next, err := s.Apply(doc)
		if err != nil {
			return nil, err
		}
		doc = next
	}
	return doc, nil
}
