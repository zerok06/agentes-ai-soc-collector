package qradar

// Offense represents a QRadar offense from the SIEM API.
type Offense struct {
	ID                         int64       `json:"id"`
	Description                string      `json:"description"`
	Severity                   int         `json:"severity"`
	Magnitude                  int         `json:"magnitude"`
	Credibility                int         `json:"credibility"`
	Relevance                  int         `json:"relevance"`
	StartTime                  int64       `json:"start_time"`
	LastUpdatedTime            int64       `json:"last_updated_time"`
	OffenseSource              string      `json:"offense_source"`
	OffenseType                int         `json:"offense_type"`
	Status                     string      `json:"status"`
	Categories                 []string    `json:"categories"`
	EventCount                 int         `json:"event_count"`
	SourceAddressIDs           []int64     `json:"source_address_ids"`
	LocalDestinationAddressIDs []int64     `json:"local_destination_address_ids"`
	LogSources                 []LogSource `json:"log_sources"`
	Rules                      []Rule      `json:"rules"`
}

// LogSource represents a log source associated with an offense.
type LogSource struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	TypeID   int64  `json:"type_id"`
	TypeName string `json:"type_name"`
}

// Rule represents a rule that contributed to an offense.
type Rule struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// ArielSearch represents the response from POST /ariel/searches and GET /ariel/searches/{id}.
type ArielSearch struct {
	SearchID     string         `json:"search_id"`
	CursorID     string         `json:"cursor_id"`
	Status       string         `json:"status"`
	Progress     int            `json:"progress"`
	RecordCount  int64          `json:"record_count"`
	ErrorMessages []ErrorMessage `json:"error_messages"`
}

// ErrorMessage represents an error from an Ariel search.
type ErrorMessage struct {
	Code     string   `json:"code"`
	Contexts []string `json:"contexts"`
	Message  string   `json:"message"`
	Severity string   `json:"severity"`
}

// ArielEvent represents a single event row from an Ariel search result.
type ArielEvent struct {
	SourceIP      string `json:"sourceip"`
	DestinationIP string `json:"destinationip"`
	Username      string `json:"username"`
	EventName     string `json:"eventname"`
	LogSource     string `json:"logsource"`
	Payload       string `json:"payload"`
}

// ArielResult wraps the result set returned by GET /ariel/searches/{id}/results.
type ArielResult struct {
	Events []ArielEvent `json:"events"`
}
