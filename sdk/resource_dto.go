package sdk

// ResourceDTO is the output-safe representation of a Resource. It carries
// only the opaque reference and the fact that opening it needs credentials;
// runtime locators, request headers, and expiry metadata never cross an output
// boundary.
type ResourceDTO struct {
	Ref                 string `json:"ref"`
	RequiresCredentials bool   `json:"requires_credentials,omitempty"`
}

// ToResourceDTO converts a runtime resource to its output-safe representation.
// A resource without an opaque reference is absent from the DTO rather than an
// invalid empty reference being emitted.
func ToResourceDTO(value Resource) *ResourceDTO {
	if value.Ref.IsZero() {
		return nil
	}
	return &ResourceDTO{
		Ref:                 value.Ref.String(),
		RequiresCredentials: value.RequiresCredentials,
	}
}
