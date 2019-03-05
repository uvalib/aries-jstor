package main

// aries is the structure of the response returned by /api/aries/:id
type aries struct {
	Identifiers []string     `json:"identifier,omitempty"`
	ServiceURL  []serviceURL `json:"service_url,omitempty"`
	AccessURL   []string     `json:"access_url,omitempty"`
}

type serviceURL struct {
	URL      string `json:"url,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// Response structure for JSTOR (admin interface)
type jstorResp struct {
	Total  int          `json:"total,omitempty"`
	Assets []jstorAsset `json:"assets,omitempty"`
}
type jstorAsset struct {
	ID               int    `json:"id,omitempty"`
	Filename         string `json:"filename,omitempty"`
	RepresentationID string `json:"representation_id,omitempty"`
}
type jstorResource struct {
	URL  string `json:"url,omitempty"`
	IIIF string `json:"iiif_url,omitempty"`
}

// Response structure for ARTSTOR (public interface)
type artstorResp struct {
	Total   int             `json:"total,omitempty"`
	Results []artstorResult `json:"results,omitempty"`
}
type artstorResult struct {
	ID        string `json:"id,omitempty"`
	ArtstorID string `json:"artstorid,omitempty"`
}
