package transformer

import (
	"fmt"
	"strings"
	"time"

	"github.com/agentes-ai/qradar-collector/internal/models"
	"github.com/agentes-ai/qradar-collector/internal/qradar"
)

// Transform maps a QRadar offense and its associated events into the output payload.
func Transform(offense *qradar.Offense, events []qradar.ArielEvent) *models.OutputPayload {
	payload := &models.OutputPayload{
		Source: "QRadar",
		Offense: models.OffenseOutput{
			Severity:    fmt.Sprintf("%d", offense.Severity),
			Magnitude:   fmt.Sprintf("%d", offense.Magnitude),
			Credibility: fmt.Sprintf("%d", offense.Credibility),
			Relevance:   fmt.Sprintf("%d", offense.Relevance),
			Description: offense.Description,
			StartTime:   fmt.Sprintf("%d", offense.StartTime/1000), // QRadar uses ms, output uses seconds
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
