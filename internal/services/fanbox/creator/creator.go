// Package creator owns normalized FANBOX creator entities. HTTP routes and
// wire response shapes live in the endpoint subpackages.
package creator

// Summary is the stable creator identity returned by list endpoints.
type Summary struct {
	ID string
}

// Creator is a normalized public creator profile. It contains no session,
// subscription secret, or endpoint-specific response envelope.
type Creator struct {
	ID                string
	DisplayName       string
	IconURL           string
	HasAdultContent   bool
	IsFollowing       bool
	CoverURL          string
	PlanFee           int
	HasSupportingPlan bool
}
