package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/frizinak/slek/slk"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/renstrom/fuzzysearch/fuzzy"
)

func (s *slek) fuzzyPath(prefix, query, suffix string) string {
	if !path.IsAbs(query) {
		return ""
	}

	ps := "/"
	reset := func(dir, fname string) {
		fpath := strings.Replace(
			strings.TrimRight(path.Clean(dir), ps)+ps+fname,
			" ",
			"\\ ",
			-1,
		)
		s.t.SetInput(
			fmt.Sprintf("%s%s %s", prefix, fpath, suffix),
			runewidth.StringWidth(prefix)+
				runewidth.StringWidth(fpath),
			0,
			false,
		)
	}

	fpath := strings.Replace(query, "\\ ", " ", -1)
	stat, err := os.Stat(fpath)
	if err == nil && !stat.IsDir() {
		// We have a file
		return fpath

	}

	if os.IsNotExist(err) || (stat != nil && stat.IsDir()) {
		fpath = strings.TrimRight(fpath, ps)
		base := path.Base(fpath)
		fpath = path.Dir(fpath)
		d, err := os.Open(fpath)
		if d != nil {
			defer d.Close()
		}

		if err != nil {
			reset(fpath, base)
			return ""
		}

		_items, err := d.Readdir(0)
		if err != nil {
			reset(fpath, base)
			return ""
		}

		items := make([]string, 0, len(_items))
		for _, f := range _items {
			fpath := path.Base(f.Name())
			if f.IsDir() {
				fpath += ps
			}

			items = append(items, fpath)
		}

		res := fuzzy.Find(base, items)

		if len(res) == 0 {
			reset(fpath, "")
			return ""
		}

		if len(res) == 1 {
			reset(fpath, res[0])
			return ""
		}

		potential := make([]string, 0)
		for i := 0; i < len(res); i++ {
			if res[i][0:len(base)] == base {
				potential = append(potential, res[i])
			}
		}

		if len(potential) == 0 {
			reset(fpath, base)
			return ""
		}

		pot := potential[0]
		for i := 1; i < len(potential); i++ {
			if len(pot) == 0 {
				break
			}
			for !strings.HasPrefix(potential[i], pot) {
				pot = pot[0 : len(pot)-1]
			}
		}

		reset(fpath, pot)
	}

	return ""
}

func (s *slek) fuzzy(
	eType slk.EntityType,
	query string,
	args []string,
) slk.Entity {
	opts := s.c.Fuzzy(eType, query)

	if eType == "" {
		s.t.Notice(fmt.Sprintf("No type '%s'", eType))
		return nil
	}

	if len(opts) == 1 {
		// autocomplete and bail
		s.t.SetInput(opts[0].GetQualifiedName()+" ", -1, -1, false)
		return opts[0]
	}

	for i := range opts {
		// multiple matches
		if query == opts[i].GetName() {
			s.t.SetInput(
				opts[0].GetQualifiedName()+" ",
				-1,
				-1,
				false,
			)
			return opts[i]
		}
	}

	if len(opts) == 0 {
		s.t.Notice(fmt.Sprintf("No such %s", eType))
	} else if len(opts) > 1 {
		names := make([]string, 0, len(opts))
		for i := range opts {
			names = append(names, opts[i].GetName())
		}
		s.t.Notice(
			fmt.Sprintf(
				"Did you mean any of: %s?",
				strings.Join(names, ", "),
			),
		)

		trgt := opts[0].GetQualifiedName()
		s.t.SetInput(
			fmt.Sprintf("%s %s", trgt, trimFields(args)),
			runewidth.StringWidth(trgt),
			0,
			false,
		)
	}

	return nil
}
