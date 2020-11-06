package etcd

type Member struct {
	// ID is the unique identifier of this Member.
	ID string `json:"id"`

	// Name is a human-readable, non-unique identifier of this Member.
	Name string `json:"name"`

	// PeerURLs represents the HTTP(S) endpoints this Member uses to
	// participate in etcd's consensus protocol.
	PeerURLs []string `json:"peerURLs"`

	// ClientURLs represents the HTTP(S) endpoints on which this Member
	// serves its client-facing APIs.
	ClientURLs []string `json:"clientURLs"`
}