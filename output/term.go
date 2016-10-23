package output

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xAX/notificator"
	"github.com/frizinak/slek/slk"
	"github.com/jroimartin/gocui"
)

type notification struct {
	channel, from, text string
}

type Term struct {
	format
	input chan string
	g     *gocui.Gui
	// Since gocui.Execute spawns goroutines
	// none of the update events are guaranteed to
	// be excuted in order.
	// Use a synchronization channel
	gQueue            chan gocui.Handler
	notifier          *notificator.Notificator
	notifyChan        chan *notification
	notificationLimit time.Duration
}

func NewTerm(
	appName,
	username string,
	notificationLimit time.Duration,
) (t *Term, input chan string) {
	input = make(chan string, 1)
	queue := make(chan gocui.Handler, 0)

	t = &Term{
		format:            format{ownUsername: username},
		input:             input,
		gQueue:            queue,
		notifier:          notificator.New(notificator.Options{AppName: appName}),
		notifyChan:        make(chan *notification, 1),
		notificationLimit: notificationLimit,
	}

	return
}

func (t *Term) Init() (err error) {
	t.g = gocui.NewGui()
	t.g.Cursor = true
	if err = t.g.Init(); err != nil {
		return
	}

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

			for _, n := range ns {
				t.Notify(n.channel, n.from, n.text, true)
			}

			ns = make(map[string]*notification, 0)
			next = time.Now().Add(t.notificationLimit)
		}

		for {
			select {
			case n := <-t.notifyChan:
				ns[n.channel] = n
				if time.Now().After(next) {
					do()
				}
			case <-time.After(t.notificationLimit):
				do()
			}
		}
	}()

	t.g.SetLayout(t.layout)

	views := []string{"input", "box", "info"}
	cview := 0

	autoScroll := func(g *gocui.Gui, v *gocui.View) error {
		v.SetOrigin(0, 0)
		v.Autoscroll = true
		return nil
	}

	submit := func(g *gocui.Gui, v *gocui.View) error {
		t.submit(g)
		return nil
	}

	scrollView := func(v *gocui.View, dy int) error {
		v.Autoscroll = false
		ox, oy := v.Origin()
		return v.SetOrigin(ox, oy+dy)
	}

	next := func(g *gocui.Gui, v *gocui.View) error {
		if cview++; cview >= len(views) {
			cview = 0
		}

		for i := range views {
			v, err := g.View(views[i])
			if err != nil {
				return err
			}
			v.Frame = i == cview

			if i != cview {
				autoScroll(g, v)
			}
		}

		_, err := g.SetCurrentView(views[cview])
		return err
	}

	quit := func(g *gocui.Gui, v *gocui.View) error {
		return gocui.ErrQuit
	}

	clear := func(g *gocui.Gui, v *gocui.View) error {
		t.SetInput("", -1, false)
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

	err = t.g.SetKeybinding("input", gocui.KeyEnter, gocui.ModNone, submit)
	if err != nil {
		return
	}

	err = t.g.SetKeybinding("input", gocui.KeyCtrlC, gocui.ModNone, clear)
	if err != nil {
		return
	}

	for _, view := range []string{"box", "info"} {
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
			_, maxY := g.Size()
			scrollView(v, -int(float64(maxY)/2))
			return nil
		}
		pdown := func(g *gocui.Gui, v *gocui.View) error {
			_, maxY := g.Size()
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

	return
}

func (t *Term) Run() error {
	defer close(t.input)
	defer t.g.Close()
	defer close(t.gQueue)

	go t.update()

	if err := t.g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}

	return nil

}

func (t *Term) Quit() {
	t.gQueue <- func(g *gocui.Gui) error {
		return gocui.ErrQuit
	}
}

func (t *Term) submit(g *gocui.Gui) {
	v, _ := g.View("input")
	t.SetInput("", -1, false)
	t.input <- v.Buffer()
}

func (t *Term) text(which, msg string) {
	t.gQueue <- func(g *gocui.Gui) error {
		v, _ := g.View(which)
		if v != nil {
			fmt.Fprint(v, msg+"\n")
		}
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

func (t *Term) boxText(msg string) {
	t.text("box", msg)
}

func (t *Term) infoText(msg string) {
	t.text("info", msg)
}

func (s *Term) Notify(channel, from, text string, force bool) {
	if !force {
		s.notifyChan <- &notification{channel, from, text}
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

	s.notifier.Push(title, strings.Join(f, " "), "", notificator.UR_NORMAL)
}

func (t *Term) Info(msg string) {
	t.infoText(t.format.Info(msg))
}

func (t *Term) Notice(msg string) {
	t.infoText(t.format.Notice(msg))
}

func (t *Term) Warn(msg string) {
	t.infoText(t.format.Warn(msg))
}

func (t *Term) Msg(channel, from, msg string, ts time.Time, section bool) {
	t.boxText(t.format.Msg(channel, from, msg, ts, section))
}

func (t *Term) File(channel, from, title, url string) {
	t.boxText(t.format.File(channel, from, title, url))
}

func (t *Term) Typing(channel, user string) {
	t.infoText(t.format.Typing(channel, user))
}

func (t *Term) Debug(msg ...string) {
	t.infoText(t.format.Debug(msg...))
}

func (t *Term) List(title string, items []*slk.ListItem) {
	t.infoText(t.format.List(title, items))
}

func (t *Term) SetInput(str string, posX int, submit bool) {
	t.g.Execute(
		func(g *gocui.Gui) error {
			v, _ := g.View("input")
			if v == nil {
				return nil
			}

			v.Clear()
			fmt.Fprint(v, str)
			// TODO, fix this shit, currently counting bytes...
			if posX < 0 {
				posX = len(str)
			}
			v.SetCursor(posX, 0)
			v.SetOrigin(0, 0)
			if submit {
				t.submit(t.g)
			}

			return nil
		},
	)
}

func (t *Term) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	maxX--

	boxW := int(2 * float64(maxX) / 3)
	if v, err := g.SetView("box", 0, 0, boxW, maxY-5); err != nil {
		v.Frame = false
		v.Autoscroll = true
		v.Wrap = true
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	if v, err := g.SetView("info", boxW+2, 0, maxX, maxY-5); err != nil {
		v.Frame = false
		v.Autoscroll = true
		v.Wrap = true
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	if v, err := g.SetView("input", 2, maxY-5, maxX-1, maxY-1); err != nil {
		v.Frame = true
		v.Editable = true
		v.Wrap = true

		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView("input"); err != nil {
			return err
		}
	}

	return nil
}
