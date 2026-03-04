package transformer

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/agentes-ai/qradar-collector/internal/models"
	"github.com/agentes-ai/qradar-collector/internal/qradar"
)

// hashRegex looks for standard hash labels in payloads (MD5, SHA1, SHA256).
var hashRegex = regexp.MustCompile(`(?i)(?:SHA256|SHA1|MD5):\s*([a-fA-F0-9]{32,64})`)

func extractHash(payload string) string {
	matches := hashRegex.FindStringSubmatch(payload)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Transform maps a QRadar offense and its associated events into the output payload.
func Transform(offense *qradar.Offense, events []qradar.ArielEvent, clientName string) *models.OutputPayload {
	payload := &models.OutputPayload{
		Source: "QRadar",
		Offense: models.OffenseOutput{
			ID:          fmt.Sprintf("%d", offense.ID),
			Client:      clientName,
			DomainID:    fmt.Sprintf("%d", offense.DomainID),
			Severity:    fmt.Sprintf("%d", offense.Severity),
			Magnitude:   fmt.Sprintf("%d", offense.Magnitude),
			Credibility: fmt.Sprintf("%d", offense.Credibility),
			Relevance:   fmt.Sprintf("%d", offense.Relevance),
			Description: offense.Description,
			StartTime:   time.Unix(offense.StartTime/1000, 0).UTC().Format(time.RFC3339),
		},
		SentAt: fmt.Sprintf("%d", time.Now().Unix()),
	}

	if len(events) > 0 {
		ev := events[0] // Use first event as primary
		payload.Event = models.EventOutput{
			SourceIP:      ev.SourceIP,
			DestinationIP: ev.DestinationIP,
			Username:      ev.Username,
			EventName:     ev.EventName,
			LogSource:     ev.LogSource,
			Payload:       ev.Payload,
			FileHash:      extractHash(ev.Payload),
		}
	} else {
		// Fallback: compose event data from offense fields.
		logSourceNames := make([]string, 0, len(offense.LogSources))
		for _, ls := range offense.LogSources {
			logSourceNames = append(logSourceNames, ls.Name)
		}

		payload.Event = models.EventOutput{
			SourceIP:      offense.OffenseSource,
			DestinationIP: "",
			Username:      "",
			EventName:     offense.Description,
			LogSource:     strings.Join(logSourceNames, ", "),
			Payload: fmt.Sprintf(
				"Offense ID: %d | Source: %s | Categories: %s | Event Count: %d",
				offense.ID,
				offense.OffenseSource,
				strings.Join(offense.Categories, ", "),
				offense.EventCount,
			),
		}
	}

	return payload
}
