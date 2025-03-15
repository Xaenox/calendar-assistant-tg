package calendar

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"calendar-assistant/pkg/openai"

	ics "github.com/arran4/golang-ical"
)

// GenerateICS generates an ICS file from an event
func GenerateICS(event *openai.Event, timezone string) ([]byte, error) {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodRequest)
	cal.SetProductId("-//Calendar Assistant//EN")

	// Validate the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Fall back to UTC if the timezone is invalid
		fmt.Printf("Invalid timezone %s, falling back to UTC\n", timezone)
		timezone = "UTC"
		loc = time.UTC
	}

	fmt.Printf("Generating ICS with timezone: %s\n", timezone)
	fmt.Printf("Original event start time (UTC): %s\n", event.StartTime.Format(time.RFC3339))
	fmt.Printf("Original event end time (UTC): %s\n", event.EndTime.Format(time.RFC3339))

	// Calculate the timezone offset
	_, offset := time.Now().In(loc).Zone()
	offsetHours := offset / 3600 // Convert seconds to hours

	fmt.Printf("Timezone offset: %d hours\n", offsetHours)

	// Adjust the times to compensate for the timezone offset
	// If GPT returns 16:00 GMT+0 and user is in GMT+3, we need to set 13:00 GMT+0
	// so that when the calendar app applies GMT+3, it will show as 16:00 GMT+3
	adjustedStartTime := event.StartTime.Add(time.Duration(-offsetHours) * time.Hour)
	adjustedEndTime := event.EndTime.Add(time.Duration(-offsetHours) * time.Hour)

	fmt.Printf("Adjusted start time (UTC): %s\n", adjustedStartTime.Format(time.RFC3339))
	fmt.Printf("Adjusted end time (UTC): %s\n", adjustedEndTime.Format(time.RFC3339))

	// Create the event
	e := cal.AddEvent(fmt.Sprintf("%d", time.Now().Unix()))
	e.SetCreatedTime(time.Now())
	e.SetDtStampTime(time.Now())
	e.SetModifiedAt(time.Now())

	// Use the adjusted times for the ICS file
	e.SetStartAt(adjustedStartTime)
	e.SetEndAt(adjustedEndTime)
	e.SetSummary(event.Title)
	e.SetDescription(event.Description)
	e.SetLocation(event.Location)

	// Add a custom property to indicate the user's display timezone
	e.AddProperty("X-DISPLAY-TIMEZONE", timezone)

	// Serialize to buffer
	var buf bytes.Buffer
	if err := cal.SerializeTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to serialize ICS: %w", err)
	}

	// Get the ICS content as string
	icsContent := buf.String()

	// For all-day events (events with time at 00:00:00), modify the format to be DATE instead of DATE-TIME
	if event.StartTime.Hour() == 0 && event.StartTime.Minute() == 0 && event.StartTime.Second() == 0 {
		fmt.Println("Detected all-day event, converting to DATE format")

		// Replace DTSTART with DATE format
		startBefore := fmt.Sprintf("DTSTART:%s", adjustedStartTime.Format("20060102T150405Z"))
		startAfter := fmt.Sprintf("DTSTART;VALUE=DATE:%s", adjustedStartTime.Format("20060102"))
		icsContent = strings.Replace(icsContent, startBefore, startAfter, -1)

		fmt.Printf("Replaced '%s' with '%s'\n", startBefore, startAfter)

		// If end time is also at midnight, replace it too
		if event.EndTime.Hour() == 0 && event.EndTime.Minute() == 0 && event.EndTime.Second() == 0 {
			endBefore := fmt.Sprintf("DTEND:%s", adjustedEndTime.Format("20060102T150405Z"))
			endAfter := fmt.Sprintf("DTEND;VALUE=DATE:%s", adjustedEndTime.Format("20060102"))
			icsContent = strings.Replace(icsContent, endBefore, endAfter, -1)

			fmt.Printf("Replaced '%s' with '%s'\n", endBefore, endAfter)
		}
	}

	fmt.Println("Final ICS content:")
	fmt.Println(icsContent)

	return []byte(icsContent), nil
}
