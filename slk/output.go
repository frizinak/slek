package slk

import (
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// ListItemStatusNone should not receive special treatment
	ListItemStatusNone = iota
	// ListItemStatusGood should indicate a positive status
	// (e.g.: user online, ...)
	ListItemStatusGood
	// ListItemStatusBad should indicate a negative status
	// (e.g.: user set to DND)
	ListItemStatusBad
	// ListItemStatusNormal should indicate a neutral status
	// (e.g.: channel exists but user is not a member)
	ListItemStatusNormal
	// ListItemStatusTitle can be used to section a list with titles.
	ListItemStatusTitle
)

// ListItemStatus allows Output implementations to differentiate
// between diffent types of ListItems.
type ListItemStatus string

// ListItem represens a single ListItems entry with a ListItemsStatus and text.
type ListItem struct {
	Status int
	Value  string
}

// ListItems is a collection items that should be rendered as a list by
// the implementation of the output interface.
type ListItems []*ListItem

func (a ListItems) Len() int      { return len(a) }
func (a ListItems) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ListItems) Less(i, j int) bool {
	if a[i].Status != a[j].Status {
		return a[i].Status < a[j].Status
	}

	if len(a[i].Value) != 0 && len(a[j].Value) != 0 {
		ib, _ := utf8.DecodeRune([]byte(a[i].Value))
		jb, _ := utf8.DecodeRune([]byte(a[j].Value))
		return runeToLower(ib) < runeToLower(jb)
	}

	return false
}

func runeToLower(r rune) rune {
	if r <= unicode.MaxASCII {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
	}

	return r
}

// Output allows for different implementations of the slk ui.
type Output interface {
	// Notify should do something that stands out relative to the rest of
	// the methods and represens a message event.
	Notify(channel, from, text string, force bool)

	// Info will be called when a 'positive' event occurs.
	Info(msg string)
	// Notice will be called when a 'neutral' event occurs.
	Notice(msg string)
	// Warn will be called when an errors occurs.
	Warn(msg string)
	// Msg should render a slack message.
	Msg(channel, from, msg string, ts time.Time, newSection bool)
	// Debug will be called with dev info.
	Debug(msg ...string)
	// Typing should notify the user of another user's typing status.
	Typing(channel, user string, timeout time.Duration)
	// File should render a file.
	File(channel, from, title, url string)
	// List should render the given list.
	List(items ListItems, reverse bool)
}
