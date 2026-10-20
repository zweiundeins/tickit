package web

// routesTracker registers projects, workflows, issues, their comments, links and history (FR-01 to FR-38, FR-65 to FR-70).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesTracker() {}
