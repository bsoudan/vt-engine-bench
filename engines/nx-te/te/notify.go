package te

import (
	"encoding/base64"
	"strings"
)

// Notification is a desktop notification a program sent its terminal: the
// program wanting the user, saying why. Three sequences say one, and a
// terminal that takes any takes the others (doc/done/ATTACH_PLAN.md, stage
// 4):
//
//   - OSC 9 ; body — iTerm2's, which Windows Terminal, kitty and WezTerm
//     take. One whose first field is a number is ConEmu's (9;4 progress,
//     9;9 the working directory, which shells send at every prompt): not a
//     notification.
//   - OSC 777 ; notify ; title ; body — urxvt's, which VTE and foot take.
//     777's other words (a shell's preexec, precmd) are not.
//   - OSC 99 ; metadata ; payload — kitty's: metadata is key=value pairs
//     split by ':'. i is the notification's id, p what the payload is
//     (title, the default, or body; close withdraws the notification of
//     that id), d=0 says more chunks of it come, e=1 that the payload is
//     base64. A query (p=?, p=alive) is not answered: a program that asks
//     hears nothing, which says notifications are not supported, and sends
//     them anyway or not.
type Notification struct {
	// ID is OSC 99's i: what a later close names. "" for 9 and 777, and
	// for a 99 that gave none.
	ID string
	// Title and Body as the program said them: OSC 9's text is a body.
	Title, Body string
	// Close says the program withdrew the notification of ID (OSC 99's
	// p=close): Title and Body are empty.
	Close bool
}

// Text is the notification in a line: its title and body joined by ": ",
// or whichever it has.
func (n Notification) Text() string {
	switch {
	case n.Title == "":
		return n.Body
	case n.Body == "":
		return n.Title
	}
	return n.Title + ": " + n.Body
}

// maxNotification caps the title and body a chunked OSC 99 builds up, each:
// a program that sends d=0 forever cannot grow the stream's memory.
const maxNotification = 4096

// notifyITerm is OSC 9's rest, the text after "9;": a notification, unless
// it is one of ConEmu's sequences, which share the number.
func (st *Stream) notifyITerm(rest string) {
	first, _, _ := strings.Cut(rest, ";")
	if first != "" && strings.Trim(first, "0123456789") == "" {
		return // ConEmu's 9;N: progress, the working directory, ...
	}
	st.listener.Notify(Notification{Body: rest})
}

// notifyURxvt is OSC 777's fields after the number: notify, the title and
// the body (which may hold ';').
func (st *Stream) notifyURxvt(fields []string) {
	if len(fields) < 2 || fields[0] != "notify" {
		return
	}
	st.listener.Notify(Notification{Title: fields[1], Body: strings.Join(fields[2:], ";")})
}

// notifyKitty is OSC 99's metadata and payload. A chunk with d=0 is kept
// (st.kitty) until the chunk that ends it, d=1 or no d, of the same id: a
// chunk of another id drops what was kept, so one notification is built at
// a time.
func (st *Stream) notifyKitty(meta, payload string) {
	var id, kind, done, enc string
	for _, kv := range strings.Split(meta, ":") {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "i":
			id = v
		case "p":
			kind = v
		case "d":
			done = v
		case "e":
			enc = v
		}
	}
	switch kind {
	case "close":
		if st.kitty != nil && st.kitty.ID == id {
			st.kitty = nil
		}
		st.listener.Notify(Notification{ID: id, Close: true})
		return
	case "?", "alive":
		return // a query, not answered (above)
	}
	if enc == "1" {
		b, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return
		}
		payload = string(b)
	}
	n := st.kitty
	if n == nil || n.ID != id {
		n = &Notification{ID: id}
	}
	switch kind {
	case "", "title":
		n.Title = capped(n.Title + payload)
	case "body":
		n.Body = capped(n.Body + payload)
	} // icon, buttons, ...: nothing shown, but they may end it
	if done == "0" {
		st.kitty = n
		return
	}
	st.kitty = nil
	if n.Title != "" || n.Body != "" {
		st.listener.Notify(*n)
	}
}

// capped is s cut to maxNotification bytes, at a rune's start.
func capped(s string) string {
	if len(s) <= maxNotification {
		return s
	}
	s = s[:maxNotification]
	for len(s) > 0 && s[len(s)-1]&0xC0 == 0x80 {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] >= 0xC0 {
		s = s[:len(s)-1] // a lead byte whose continuation was cut
	}
	return s
}
