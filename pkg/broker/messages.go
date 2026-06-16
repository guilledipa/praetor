package broker

type CatalogRequest struct {
	NodeID string            `json:"node_id"`
	Facts  map[string]string `json:"facts"`
}

type CatalogResponse struct {
	Catalog            string `json:"catalog"`
	Signature          []byte `json:"signature"`
	SignatureAlgorithm string `json:"signature_algorithm"`
}

type ResourceReport struct {
	Type      string `json:"type"`
	Id        string `json:"id"`
	Compliant bool   `json:"compliant"`
	Message   string `json:"message"`
}

type StateReportRequest struct {
	NodeID      string            `json:"node_id"`
	Resources   []*ResourceReport `json:"resources"`
	IsCompliant bool              `json:"is_compliant"`
	Timestamp   int64             `json:"timestamp"`
}

type StateReportResponse struct {
	Acknowledged bool `json:"acknowledged"`
}
