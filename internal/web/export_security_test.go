package web

import "github.com/zweiundeins/lith/edge"

// Unlimited lifts the rate limits of actions, sign-ins and streams, for
// the security tests that send every action as every kind of person: a
// 429 would hide the 403 they look for (DM-04 has tests of its own).
func Unlimited(s *Server) {
	free := func() *edge.Limiter { return edge.NewLimiter(1e6, 1e6) }
	l := &limits{site: free(), classes: map[string]limiter{}}
	for _, c := range classes {
		l.classes[c.name] = limiter{byAddr: free(), bySession: free()}
	}
	s.limits, s.signIn, s.streams = l, free(), newStreamLimits()
	s.streams.opens = free()
}
