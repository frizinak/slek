package output

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/frizinak/gnotifier"
	"github.com/frizinak/gocui"
	"github.com/frizinak/slek/slk"
	"github.com/mattn/go-runewidth"
)

const (
	viewEvent  = "event"
	viewInfo   = "info"
	viewChat   = "chat"
	viewInput  = "input"
	viewTyping = "typing"
)

type notification struct {
	channel, from, text string
	created             time.Time
	sent                bool
}

type view struct {
	name  string
	frame bool
}

// Term is a gocui / termbox slk.Output implementation that allows
// for user input which is communicated over
// the input channel as returned by NewTerm.
type Term struct {
	format
	appName string
	appIcon string
	input   chan string
	g       *gocui.Gui
	// Since gocui.Execute spawns goroutines
	// none of the update events are guaranteed to
	// be excuted in order.
	// Use a synchronization channel
	gQueue              chan func(*gocui.Gui) error
	notifyChan          chan *notification
	notificationLimit   time.Duration
	notificationTimeout time.Duration

	clearTypingMutex sync.Mutex
	clearTypingBox   *time.Time

	resetEventMutex sync.Mutex
	resetEventBox   *time.Time
	eventBoxCache   string

	//boxWidth    uint
	//infoWidth   uint
	//typingWidth uint
	dimensions map[string]uint
	views      []*view
}

// NewTerm returns a Term and an input channel which will receive the current
// input field contents when it is 'submitted'.
func NewTerm(
	appName,
	appIcon,
	username string,
	timeFormat string,
	notificationLimit time.Duration,
	notificationTimeout time.Duration,
) (t *Term, input chan string) {
	input = make(chan string, 1)
	queue := make(chan func(*gocui.Gui) error, 10) // Allow 10 recursive updates? :s

	t = &Term{
		format: format{
			ownUsername: username,
			timeFormat:  timeFormat,
		},
		appName:             appName,
		appIcon:             appIcon,
		input:               input,
		gQueue:              queue,
		notifyChan:          make(chan *notification, 1),
		notificationLimit:   notificationLimit,
		notificationTimeout: notificationTimeout,
		dimensions:          map[string]uint{},
		views: []*view{
			{viewEvent, false},
			{viewInfo, true},
			{viewChat, true},
			{viewInput, true},
			{viewTyping, false},
		},
	}

	return
}

// Init sets up the gocui / termbox environment.
func (t *Term) Init() (err error) {
	if t.g, err = gocui.NewGui(); err != nil {
		return
	}

	t.g.Cursor = true

	defer func() {
		if err != nil {
			t.g.Close()
		}
	}()

	go func() {
		ns := make(map[string]*notification, 0)
		var next time.Time
		do := func() {
			if len(ns) == 0 {
				return
			}

			for i, n := range ns {
				if n.sent {
					continue
				}

				t.Notify(n.channel, n.from, n.text, true)
				// Empty from, channel and text fields.
				ns[i] = &notification{sent: true, created: n.created}
			}

			next = time.Now().Add(t.notificationLimit)
		}

		for {
			select {
			case n := <-t.notifyChan:
				if last, ok := ns[n.channel]; ok &&
					last.sent &&
					n.created.Sub(last.created) < t.notificationLimit {
					break
				}

				ns[n.channel] = n
				if time.Now().After(next) {
					do()
				}
			case <-time.After(time.Second):
				do()
			}
		}
	}()

	t.g.SetManagerFunc(t.layout)

	views := []string{viewInput, viewChat}
	cview := 0

	autoScroll := func(g *gocui.Gui, v *gocui.View) error {
		v.SetOrigin(0, 0)
		v.Autoscroll = true
		return nil
	}

	submit := func(g *gocui.Gui, v *gocui.View) error {
		t.submit()
		return nil
	}

	scrollView := func(v *gocui.View, dy int) error {
		v.Autoscroll = false
		ox, oy := v.Origin()
		y := oy + dy
		if y < 0 {
			y = 0
		}
		return v.SetOrigin(ox, y)
	}

	next := func(g *gocui.Gui, v *gocui.View) error {
		if g.CurrentView().Name() == viewInfo {
			cview--
		}

		if cview++; cview >= len(views) {
			cview = 0
		}

		t.setActive(views[cview])
		return nil
	}

	quit := func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}

	clear := func(g *gocui.Gui, v *gocui.View) error {
		t.SetInput("", -1, -1, false)
		return nil
	}

	err = t.g.SetKeybinding("", gocui.KeyCtrlQ, gocui.ModNone, quit)
	if err != nil {
		return
	}

	err = t.g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, next)
	if err != nil {
		return
	}

	err = t.g.SetKeybinding(viewInput, gocui.KeyEnter, gocui.ModNone, submit)
	if err != nil {
		return
	}

	err = t.g.SetKeybinding(viewInput, gocui.KeyCtrlC, gocui.ModNone, clear)
	if err != nil {
		return
	}

	for _, view := range []string{viewChat, viewInfo} {
		maxScroll := func(g *gocui.Gui, v *gocui.View, intent int) int {
			if intent < 0 {
				// termbox already protects against scrolling up to far
				return intent
			}

			// TODO it seems ViewBuffer and Buffer return the entire buffer
			// it is possible to stop overscrolling (origin.Y + totalLines > 0)
			// but both of those calls are a ridiculous memory hog.
			// so screw it.
			return intent

			// :( no access to the lines slice (viewLines)
			lines := len(strings.Split(v.ViewBuffer(), "\n")) - 1
			// Wait wut. ViewBuffer doesn't return the lines
			// currently visible?? what am I missing here?
			if intent > lines {
				return lines
			}

			return intent
		}

		up := func(g *gocui.Gui, v *gocui.View) error {
			scrollView(v, -1)
			return nil
		}
		down := func(g *gocui.Gui, v *gocui.View) error {
			scrollView(v, maxScroll(g, v, 1))
			return nil
		}

		pup := func(g *gocui.Gui, v *gocui.View) error {
			_, maxY := v.Size()
			scrollView(v, -int(float64(maxY)/2))
			return nil
		}
		pdown := func(g *gocui.Gui, v *gocui.View) error {
			_, maxY := v.Size()
			scrollView(v, maxScroll(g, v, int(float64(maxY)/2)))
			return nil
		}

		// TODO remove, use maxScroll == 0 as a trigger to
		// set AutoScroll to true. well when maxScroll works, that is.
		err = t.g.SetKeybinding(view, 'z', gocui.ModNone, autoScroll)
		if err != nil {
			return
		}

		err = t.g.SetKeybinding(view, gocui.KeyArrowUp, gocui.ModNone, up)
		if err != nil {
			return
		}
		err = t.g.SetKeybinding(view, gocui.KeyArrowDown, gocui.ModNone, down)
		if err != nil {
			return err
		}

		err = t.g.SetKeybinding(view, 'k', gocui.ModNone, up)
		if err != nil {
			return
		}
		err = t.g.SetKeybinding(view, 'j', gocui.ModNone, down)
		if err != nil {
			return err
		}

		err = t.g.SetKeybinding(view, gocui.KeyCtrlU, gocui.ModNone, pup)
		if err != nil {
			return
		}
		err = t.g.SetKeybinding(view, gocui.KeyCtrlD, gocui.ModNone, pdown)
		if err != nil {
			return err
		}
	}

	t.setActive(viewInput)

	return
}

// SetUsername sets the current user's name so the formatter can make it
// stand out. Also disables notifications if messages are received from
// this user.
func (t *Term) SetUsername(username string) {
	t.format.setUsername(username)
}

// BindKey allows binding a gocui.Key-press to the given handler.
func (t *Term) BindKey(key gocui.Key, handler func() error) error {
	h := func(g *gocui.Gui, v *gocui.View) error {
		return handler()
	}

	return t.g.SetKeybinding(viewInput, key, gocui.ModNone, h)
}

// Run starts the gocui and Term event loop.
func (t *Term) Run() error {
	defer close(t.input)
	defer t.g.Close()
	defer close(t.gQueue)

	go t.update()

	// TODO Quit should cancel these:
	go func() {
		for {
			t.clearTypingMutex.Lock()
			now := time.Now()
			clear := t.clearTypingBox != nil && t.clearTypingBox.Before(now)
			if !clear {
				t.clearTypingMutex.Unlock()
				time.Sleep(time.Millisecond * 200)
				continue
			}

			t.clearTypingBox = nil
			t.clearTypingMutex.Unlock()

			t.gQueue <- func(g *gocui.Gui) error {
				v, _ := g.View(viewTyping)
				v.Clear()
				return nil
			}
		}
	}()

	go func() {
		for {
			t.resetEventMutex.Lock()
			now := time.Now()
			reset := t.resetEventBox != nil && t.resetEventBox.Before(now)
			if !reset {
				t.resetEventMutex.Unlock()
				time.Sleep(time.Millisecond * 200)
				continue
			}

			t.resetEventBox = nil
			t.resetEventMutex.Unlock()

			t.eventText(t.eventBoxCache, 0)
		}
	}()

	if err := t.g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}

	return nil

}

// Quit should stop the event loop and return the terminal to
// a usable state.
// TODO block until we've actually quit.
func (t *Term) Quit() {
	t.gQueue <- func(g *gocui.Gui) error {
		return gocui.ErrQuit
	}
}

func (t *Term) Notify(channel, from, text string, force bool) {
	if !force {
		t.notifyChan <- &notification{channel, from, text, time.Now(), false}
		return
	}

	title := from
	if from != channel {
		title = fmt.Sprintf("%s: %s", channel, from)
	}

	f := strings.Fields(strings.Split(strings.TrimSpace(text), "\n")[0])

	l := 0
	for i := 0; i < len(f); i++ {
		l += len(f[i])
		if l > 100 {
			f = f[0:i]
			break
		}
	}

	msg := strings.Join(f, " ")
	err := gnotifier.Notify(
		t.appName,
		title,
		msg,
		t.notificationTimeout,
		t.appIcon,
	)

	if err != nil {
		t.Warn(fmt.Sprintf("Failed to trigger notification: [%s] %s", title, msg))
	}
}

func (t *Term) Info(msg string) {
	t.eventText(t.format.Info(msg), 0)
}

func (t *Term) Notice(msg string) {
	t.eventText(t.format.Notice(msg), time.Second*3)
}

func (t *Term) Warn(msg string) {
	t.eventText(t.format.Warn(msg), time.Second*3)
}

func (t *Term) Msg(channel, from, msg string, ts time.Time, section bool) {
	t.boxText(t.format.Msg(channel, from, msg, ts, section))
}

func (t *Term) File(channel, from, title, url string) {
	t.boxText(t.format.File(channel, from, title, url))
}

func (t *Term) Typing(channel, user string, timeout time.Duration) {
	t.typingText(t.format.Typing(channel, user), timeout)
}

func (t *Term) Debug(msg ...string) {
	t.format.Debug(msg...)
}

func (t *Term) List(list slk.ListItems, reverse bool) {
	t.infoText(t.format.List(list, reverse))
}

// Meta like List writes its output to the info view.
func (t *Term) Meta(str string) {
	t.infoText(str)
}

// Input returns the contents of the input field.
func (t *Term) Input() string {
	v, _ := t.g.View(viewInput)
	return v.Buffer()
}

// Clear the chat view
func (t *Term) Clear() {
	t.gQueue <- func(g *gocui.Gui) error {
		v, err := g.View(viewChat)
		if err != nil {
			return err
		}

		v.Clear()
		v.SetOrigin(0, 0)
		return nil
	}
}

// SetInput overwrites the current input field and sets the cursor
// to posX x posY.
//
// If posY == -1 the cursor will be set to the last line.
//
// If posX == -1 the cursor will be set the last column of the current line.
//
// If submit == true the contents will be send to the input channel.
func (t *Term) SetInput(str string, posX int, posY int, submit bool) {
	t.gQueue <- func(g *gocui.Gui) error {
		v, err := g.View(viewInput)
		if err != nil {
			return err
		}

		v.Clear()
		fmt.Fprint(v, str)

		parts := strings.Split(str, "\n")
		var line string
		if posY < 0 {
			posY = len(parts) - 1
		}

		if posY < len(parts) {
			line = parts[posY]
		}

		if posX < 0 {
			posX = runewidth.StringWidth(line)
		}

		v.SetOrigin(0, posY)
		v.SetCursor(posX, 0)
		if submit {
			t.submit()
		}

		return nil
	}
}

func (t *Term) setActive(which string) {
	t.gQueue <- func(g *gocui.Gui) error {
		for _, v := range t.views {
			view, err := g.View(v.name)
			if err != nil {
				return err
			}

			view.Frame = v.name == which && v.frame
			if v.name != which {
				view.SetOrigin(0, 0)
				view.Autoscroll = v.name != viewInfo
			}
		}

		if which != viewInfo {
			g.SetViewOnTop(viewChat)
		}

		if _, err := g.SetViewOnTop(which); err != nil {
			return err
		}
		_, err := g.SetCurrentView(which)
		return err
	}
}

func (t *Term) submit() {
	t.gQueue <- func(g *gocui.Gui) error {
		v, _ := g.View(viewInput)
		t.input <- v.Buffer()
		t.SetInput("", -1, -1, false)
		return nil
	}
}

func (t *Term) update() {
	// TODO batch process items, with a timeout of 30 fps or sumn?
	var wg sync.WaitGroup
	for task := range t.gQueue {
		wg.Add(1)
		t.g.Execute(
			func(g *gocui.Gui) error {
				defer wg.Done()
				return task(g)
			},
		)

		wg.Wait()
	}
}

func (t *Term) text(which, msg string, clear bool) {
	t.gQueue <- func(g *gocui.Gui) error {
		v, _ := g.View(which)
		if v != nil {
			if clear {
				v.Clear()
			}

			if width, ok := t.dimensions[which]; ok {
				msg = t.wrap(msg, width)
			}

			fmt.Fprint(v, msg+"\n")
		}

		if which == viewInfo {
			t.setActive(which)
		}

		return nil
	}
}

func (t *Term) boxText(msg string) {
	t.text(viewChat, msg, false)
}

func (t *Term) infoText(msg string) {
	t.text(viewInfo, msg, true)
}

func (t *Term) eventText(msg string, timeout time.Duration) {
	if timeout == 0 {
		t.eventBoxCache = msg
	} else {
		at := time.Now().Add(timeout)
		t.resetEventMutex.Lock()
		defer t.resetEventMutex.Unlock()
		if t.resetEventBox == nil || t.resetEventBox.Before(at) {
			t.resetEventBox = &at
		}
	}

	t.text(viewEvent, msg, true)
}

func (t *Term) typingText(msg string, timeout time.Duration) {
	at := time.Now().Add(timeout)
	t.clearTypingMutex.Lock()
	defer t.clearTypingMutex.Unlock()
	if t.clearTypingBox == nil || t.clearTypingBox.Before(at) {
		t.clearTypingBox = &at
	}

	t.text(viewTyping, msg, true)
}

func (t *Term) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	maxX--

	t.dimensions[viewChat] = uint(maxX - 2)
	if v, err := g.SetView(viewChat, 0, 3, maxX, maxY-5); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = false
		v.Autoscroll = true
		v.Wrap = true
	}

	t.dimensions[viewInfo] = t.dimensions[viewChat] - 20
	if v, err := g.SetView(viewInfo, 10, 4, maxX-10, maxY-3); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = true
		v.Autoscroll = false
		v.Wrap = true
	}

	if v, err := g.SetView(viewInput, 0, maxY-5, maxX, maxY-2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = true
		v.Editable = true
		v.Wrap = false
		v.Editor = gocui.EditorFunc(editor)
	}

	t.dimensions[viewEvent] = t.dimensions[viewChat]
	if v, err := g.SetView(viewEvent, 0, 0, maxX, 4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = false
		v.Wrap = false
		v.Autoscroll = false
	}

	t.dimensions[viewTyping] = t.dimensions[viewChat]
	if v, err := g.SetView(viewTyping, 0, maxY-2, maxX, maxY+1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Frame = false
		v.Wrap = false
		v.Autoscroll = true
	}

	return nil
}

func editor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyDelete:
		v.EditDelete(false)
	case key == gocui.KeyInsert:
		v.Overwrite = !v.Overwrite
	case key == gocui.KeyArrowDown:
		move(v, 0, 1)
	case key == gocui.KeyArrowUp:
		move(v, 0, -1)
	case key == gocui.KeyArrowLeft:
		move(v, -1, 0)
	case key == gocui.KeyArrowRight:
		move(v, 1, 0)
	}

}

func move(v *gocui.View, dx, dy int) {
	orx, ory := v.Origin()
	ox, oy := v.Cursor()
	ox += orx
	oy += ory

	x, y := ox+dx, oy+dy

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	line, _ := v.Line(y)
	cols := runewidth.StringWidth(line)
	if x > cols {
		x = cols
		if dy == 0 {
			return
		}
	}

	v.MoveCursor(x-ox, y-oy, true)
}
