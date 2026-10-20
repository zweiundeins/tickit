package web

// routesEditor registers the rich-text editor: edit batches, carets, typing indicators (FR-82, FR-86, MP-05 to MP-07).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesEditor() {}
