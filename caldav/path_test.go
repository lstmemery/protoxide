package caldav

import (
	"context"
	"strings"
	"testing"
)

// Regression: request paths that don't sit under the calendar home set used to
// parse into an empty calendar ID ("") — the backend then keyed phantom cache
// and poller state on "" and 404-spammed Proton's API for the life of the
// process (observed via GET /api/calendar/v1//modelevents/latest in the logs).
// All accessors must reject such paths up front instead of silently accepting
// them. The guards must check provenance (that the home-set prefix actually
// matched), not just segment shape: relative hrefs — reachable via
// calendar-multiget, which forwards href.Path from client XML — parse to
// well-shaped paths that would key phantom state on a crafted calendar ID.
func TestPathsOutsideHomeSetAreRejected(t *testing.T) {
	b := &backend{}
	ctx := context.Background()

	objectPaths := []string{
		"/caldav/watchdog-probe",     // observed in production logs
		"/caldav/calendars",          // home set, unslashed
		"/caldav/calendars/",         // home set itself
		"/caldav/calendars//obj.ics", // empty calendar ID
		"/caldav/calendars/cal1/",    // collection, not an object
		"",                           // empty path
		"/nowhere/else/event.ics",    // different tree
		"cal1/review-event.ics",      // relative href (multiget route)
	}
	for _, path := range objectPaths {
		if _, err := b.GetCalendarObject(ctx, path, nil); !rejected(err) {
			t.Errorf("GetCalendarObject(%q): want home-set rejection, got %v", path, err)
		}
		if _, err := b.PutCalendarObject(ctx, path, nil, nil); !rejected(err) {
			t.Errorf("PutCalendarObject(%q): want home-set rejection, got %v", path, err)
		}
		if err := b.DeleteCalendarObject(ctx, path); !rejected(err) {
			t.Errorf("DeleteCalendarObject(%q): want home-set rejection, got %v", path, err)
		}
	}

	// List/Query take a calendar path (not an object path), so a well-formed
	// calendar path like /caldav/calendars/cal1/ passes the guard by design and
	// needs a live backend — only the malformed shapes are asserted here.
	calendarPaths := []string{
		"/caldav/watchdog-probe",
		"/caldav/calendars",
		"/caldav/calendars/",
		"/caldav/calendars//obj.ics",
		"",
		"/nowhere/else/event.ics",
		"cal1", // relative calendar path (multiget route)
	}
	for _, path := range calendarPaths {
		if _, err := b.ListCalendarObjects(ctx, path, nil); !rejected(err) {
			t.Errorf("ListCalendarObjects(%q): want home-set rejection, got %v", path, err)
		}
		if _, err := b.QueryCalendarObjects(ctx, path, nil); !rejected(err) {
			t.Errorf("QueryCalendarObjects(%q): want home-set rejection, got %v", path, err)
		}
	}
}

// rejected reports whether err is one of the home-set guards. The guards can't
// be matched by status code: go-webdav's HTTPError type is unexported and the
// internal package that defines it is not importable from outside the library.
func rejected(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not a calendar")
}
