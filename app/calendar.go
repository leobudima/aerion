package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/logging"
)

const (
	googleCalendarEventsReadonlyScope = "https://www.googleapis.com/auth/calendar.events.readonly"
	googleCalendarReadonlyScope       = "https://www.googleapis.com/auth/calendar.readonly"
)

// CalendarEvent is the frontend-facing representation of a Google Calendar event.
type CalendarEvent struct {
	ID             string             `json:"id"`
	Summary        string             `json:"summary"`
	Description    string             `json:"description,omitempty"`
	Location       string             `json:"location,omitempty"`
	Start          time.Time          `json:"start"`
	End            time.Time          `json:"end"`
	AllDay         bool               `json:"allDay"`
	HTMLLink       string             `json:"htmlLink"`
	HangoutLink    string             `json:"hangoutLink,omitempty"`
	Organizer      CalendarPerson     `json:"organizer"`
	Creator        CalendarPerson     `json:"creator"`
	Attendees      []CalendarAttendee `json:"attendees,omitempty"`
	RecurringEvent bool               `json:"recurringEvent"`
	Recurrence     []string           `json:"recurrence,omitempty"`
	ReminderText   string             `json:"reminderText,omitempty"`
	AccountID      string             `json:"accountId"`
	AccountName    string             `json:"accountName"`
	AccountEmail   string             `json:"accountEmail"`
}

type CalendarPerson struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Self  bool   `json:"self,omitempty"`
}

type CalendarAttendee struct {
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
}

type googleCalendarEventsResponse struct {
	Items []googleCalendarEvent `json:"items"`
}

type googleCalendarEvent struct {
	ID               string                      `json:"id"`
	Summary          string                      `json:"summary"`
	Description      string                      `json:"description"`
	Location         string                      `json:"location"`
	Start            googleCalendarEventTime     `json:"start"`
	End              googleCalendarEventTime     `json:"end"`
	HTMLLink         string                      `json:"htmlLink"`
	HangoutLink      string                      `json:"hangoutLink"`
	Organizer        googleCalendarPerson        `json:"organizer"`
	Creator          googleCalendarPerson        `json:"creator"`
	Attendees        []googleCalendarAttendee    `json:"attendees"`
	RecurringEventID string                      `json:"recurringEventId"`
	Recurrence       []string                    `json:"recurrence"`
	Reminders        googleCalendarEventReminder `json:"reminders"`
}

type googleCalendarEventTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type googleCalendarPerson struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Self        bool   `json:"self"`
}

type googleCalendarAttendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"displayName"`
	ResponseStatus string `json:"responseStatus"`
	Optional       bool   `json:"optional"`
	Organizer      bool   `json:"organizer"`
	Self           bool   `json:"self"`
}

type googleCalendarEventReminder struct {
	UseDefault bool                     `json:"useDefault"`
	Overrides  []googleCalendarReminder `json:"overrides"`
}

type googleCalendarReminder struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

// ListUpcomingCalendarEvents returns upcoming events from a Google OAuth account's primary calendar.
func (a *App) ListUpcomingCalendarEvents(accountID string, maxResults int) ([]CalendarEvent, error) {
	log := logging.WithComponent("app.calendar")

	acc, err := a.resolveCalendarAccount(accountID)
	if err != nil {
		return nil, err
	}

	if acc.AuthType != account.AuthOAuth2 || !strings.Contains(strings.ToLower(acc.IMAPHost), "gmail") {
		return nil, fmt.Errorf("calendar is only available for Google OAuth accounts")
	}

	tokens, err := a.credStore.GetOAuthTokens(acc.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth tokens: %w", err)
	}
	if !hasAnyScope(tokens.Scopes, googleCalendarEventsReadonlyScope, googleCalendarReadonlyScope) {
		return nil, fmt.Errorf("Google Calendar permission is missing; sign in again to grant calendar access")
	}

	tokens, err = a.getValidOAuthToken(acc.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid OAuth token: %w", err)
	}

	if maxResults <= 0 {
		maxResults = 20
	}
	if maxResults > 50 {
		maxResults = 50
	}

	params := url.Values{}
	params.Set("timeMin", time.Now().UTC().Format(time.RFC3339))
	params.Set("singleEvents", "true")
	params.Set("orderBy", "startTime")
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

	endpoint := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read calendar response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Str("body", string(body)).Msg("Google Calendar request failed")
		return nil, fmt.Errorf("Google Calendar request failed with status %d", resp.StatusCode)
	}

	var calendarResp googleCalendarEventsResponse
	if err := json.Unmarshal(body, &calendarResp); err != nil {
		return nil, fmt.Errorf("failed to parse calendar response: %w", err)
	}

	events := make([]CalendarEvent, 0, len(calendarResp.Items))
	for _, item := range calendarResp.Items {
		start, allDay := parseGoogleCalendarTime(item.Start)
		end, _ := parseGoogleCalendarTime(item.End)
		events = append(events, CalendarEvent{
			ID:             item.ID,
			Summary:        item.Summary,
			Description:    item.Description,
			Location:       item.Location,
			Start:          start,
			End:            end,
			AllDay:         allDay,
			HTMLLink:       item.HTMLLink,
			HangoutLink:    item.HangoutLink,
			Organizer:      calendarPersonFromGoogle(item.Organizer),
			Creator:        calendarPersonFromGoogle(item.Creator),
			Attendees:      calendarAttendeesFromGoogle(item.Attendees),
			RecurringEvent: item.RecurringEventID != "" || len(item.Recurrence) > 0,
			Recurrence:     item.Recurrence,
			ReminderText:   formatReminderText(item.Reminders),
			AccountID:      acc.ID,
			AccountName:    acc.Name,
			AccountEmail:   acc.Email,
		})
	}

	return events, nil
}

func calendarPersonFromGoogle(person googleCalendarPerson) CalendarPerson {
	return CalendarPerson{
		Email: person.Email,
		Name:  person.DisplayName,
		Self:  person.Self,
	}
}

func calendarAttendeesFromGoogle(googleAttendees []googleCalendarAttendee) []CalendarAttendee {
	if len(googleAttendees) == 0 {
		return nil
	}
	attendees := make([]CalendarAttendee, 0, len(googleAttendees))
	for _, attendee := range googleAttendees {
		attendees = append(attendees, CalendarAttendee{
			Email:          attendee.Email,
			Name:           attendee.DisplayName,
			ResponseStatus: attendee.ResponseStatus,
			Optional:       attendee.Optional,
			Organizer:      attendee.Organizer,
			Self:           attendee.Self,
		})
	}
	return attendees
}

func formatReminderText(reminders googleCalendarEventReminder) string {
	if reminders.UseDefault {
		return "Default reminders"
	}
	if len(reminders.Overrides) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reminders.Overrides))
	for _, reminder := range reminders.Overrides {
		if reminder.Minutes < 60 {
			parts = append(parts, fmt.Sprintf("%d minutes before", reminder.Minutes))
			continue
		}
		hours := reminder.Minutes / 60
		if hours < 24 {
			parts = append(parts, fmt.Sprintf("%d hours before", hours))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d days before", hours/24))
	}
	return strings.Join(parts, ", ")
}

func (a *App) resolveCalendarAccount(accountID string) (*account.Account, error) {
	if accountID != "" && accountID != "unified" {
		acc, err := a.accountStore.Get(accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get account: %w", err)
		}
		if acc == nil {
			return nil, fmt.Errorf("account not found: %s", accountID)
		}
		return acc, nil
	}

	accounts, err := a.accountStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	for _, acc := range accounts {
		if acc.AuthType == account.AuthOAuth2 && strings.Contains(strings.ToLower(acc.IMAPHost), "gmail") {
			return acc, nil
		}
	}

	return nil, fmt.Errorf("no Google OAuth account found")
}

func hasAnyScope(scopes []string, needles ...string) bool {
	for _, scope := range scopes {
		for _, needle := range needles {
			if scope == needle {
				return true
			}
		}
	}
	return false
}

func parseGoogleCalendarTime(value googleCalendarEventTime) (time.Time, bool) {
	if value.DateTime != "" {
		parsed, err := time.Parse(time.RFC3339, value.DateTime)
		if err == nil {
			return parsed, false
		}
	}
	if value.Date != "" {
		parsed, err := time.Parse("2006-01-02", value.Date)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
