package web

// routesAdmin registers administration: accounts, site roles, statistics and the reset (FR-140 to FR-144).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesAdmin() {}
