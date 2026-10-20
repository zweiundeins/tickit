package web

// routesWiki registers spaces, pages, versions, page comments, templates and the trash (FR-75 to FR-105).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesWiki() {}
