package files

// ListingSnapshot is an optional retained paths hint, never an authoritative
// listing or current Git status. An absent Paths means no snapshot is available.
type ListingSnapshot struct {
	Paths      *Listing `json:"paths,omitempty"`
	Generation string   `json:"generation,omitempty"`
	Unverified bool     `json:"unverified"`
}
