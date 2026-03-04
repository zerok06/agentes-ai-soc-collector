package models

// OutputPayload is the exact JSON structure sent to the external ingestion API.
type OutputPayload struct {
	Source  string        `json:"source"`
	Offense OffenseOutput `json:"offense"`
	Event   EventOutput   `json:"event"`
	SentAt  string        `json:"sent_at"`
}

// OffenseOutput holds transformed offense fields.
type OffenseOutput struct {
	ID          string `json:"id"`
	Client      string `json:"client"`
	DomainID    string `json:"domain_id"`
	Severity    string `json:"severity"`
	Magnitude   string `json:"magnitude"`
	Credibility string `json:"credibility"`
	Relevance   string `json:"relevance"`
	Description string `json:"description"`
	StartTime   string `json:"start_time"`
}

// EventOutput holds transformed event fields.
type EventOutput struct {
	SourceIP      string `json:"source_ip"`
	DestinationIP string `json:"destination_ip"`
	Username      string `json:"username"`
	EventName     string `json:"event_name"`
	LogSource     string `json:"log_source"`
	Payload       string `json:"payload"`
	FileHash      string `json:"file_hash,omitempty"`
}
