package lambda

import "testing"

func TestSelectRoute(t *testing.T) {
	cases := []struct {
		name     string
		isLambda bool
		mode     string
		want     Route
	}{
		{"not in lambda runs cli", false, "interactions", RouteCLI},
		{"not in lambda ignores mode", false, "", RouteCLI},
		{"interactions mode", true, "interactions", RouteInteractions},
		{"send mode", true, "send", RouteSend},
		{"unknown mode", true, "bogus", RouteUnknown},
		{"empty mode in lambda", true, "", RouteUnknown},
		{"whitespace trimmed", true, " send ", RouteSend},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SelectRoute(c.isLambda, c.mode); got != c.want {
				t.Errorf("SelectRoute(%v, %q) = %d, want %d", c.isLambda, c.mode, got, c.want)
			}
		})
	}
}
