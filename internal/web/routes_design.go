package web

// routesDesign registers the design system's page, /design, which renders every component (docs/architecture.md, Frontend).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesDesign() { s.designRoutes() }
