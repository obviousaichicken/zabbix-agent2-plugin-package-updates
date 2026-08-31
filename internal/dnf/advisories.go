package dnf

import (
	"context"
)

// SecurityAdvisories returns applicable security advisories independently of
// the package snapshot.
func (c *Client) SecurityAdvisories(ctx context.Context) (AdvisoryData, error) {
	capabilities, err := c.capabilities(ctx)
	if err != nil {
		return AdvisoryData{}, err
	}
	if capabilities.DNF5 {
		return c.securityAdvisoriesDNF5(ctx, capabilities.Version)
	}

	return c.securityAdvisoriesDNF4(ctx)
}
