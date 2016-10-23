package slk

import (
	"time"
	"unicode"
)

const (
	ListItemStatusNone = iota
	ListItemStatusGood
	ListItemStatusBad
	ListItemStatusNormal
)

type ListItemStatus string

type ListItem struct {
	Status int
	Value  string
}

type ListItems []*ListItem

func (a ListItems) Len() int      { return len(a) }
func (a ListItems) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ListItems) Less(i, j int) bool {
	if a[i].Status != a[j].Status {
		return a[i].Status < a[j].Status
	}

	// TODO multibyte
	if len(a[i].Value) != 0 && len(a[j].Value) != 0 {
		return byteToLower(a[i].Value[0]) < byteToLower(a[j].Value[0])
	}

	return false
}

func byteToLower(r byte) byte {
	if r <= unicode.MaxASCII {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
	}

	return r
}

type Output interface {
	Notify(channel, from, text string, force bool)

	Info(msg string)
	Notice(msg string)
	Warn(msg string)
	Msg(channel, from, msg string, ts time.Time, newSection bool)
	Debug(msg ...string)
	Typing(channel, user string)
	File(channel, from, title, url string)
	List(title string, items []*ListItem)
}
