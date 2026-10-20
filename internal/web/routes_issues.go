package web

// routesIssues registers the issue list, its filters, saved filters, bulk edit and dashboards (FR-50 to FR-61).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesIssues() {}
