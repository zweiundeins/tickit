package web

// routesBoard registers the board, the backlog and sprints (FR-40 to FR-46).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesBoard() {}
