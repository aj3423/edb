package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/c-bata/go-prompt"
)

type cmd interface {
	Match(args []string) Matches
}

// executable commands
type executableCmd interface {
	cmd
	Exec()
}

// match result
type Match struct {
	cmd
	isPartial bool
	*prompt.Suggest
}
type Matches []Match

type Sub []cmd

func (n *Sub) Match(args []string) (matches Matches) {
	for _, sub := range *n {
		matches = append(matches, sub.Match(args)...)
	}
	return matches
}

type Cmd struct {
	name string
	help string
	exec func()
	*Sub // sub command list
}

func (c *Cmd) Name() string {
	return c.name
}
func (c *Cmd) Help() string {
	return c.help
}

func (c *Cmd) Match(args []string) (matches Matches) {
	size := len(args)
	if size == 0 { // no arg left, no match
		return
	}
	if args[0] == c.name { // fully match for current arg
		if size == 1 {
			return Matches{{c, false, &prompt.Suggest{c.name, c.help}}}
		} else { // more sub commands
			return c.Sub.Match(args[1:])
		}
	}
	if size == 1 && strings.HasPrefix(c.name, args[0]) { // partially match
		return Matches{{c, true, &prompt.Suggest{c.name, c.help}}}
	}
	return
}
func (n *Cmd) Exec() {
	n.exec()
}

type Value[T any] struct {
	val  T
	exec func(T)
}

type IntValue Value[uint64]

func (n *IntValue) Match(args []string) (matches Matches) {
	if len(args) != 1 || args[0] == "" {
		return
	}
	var e error
	n.val, e = parse_any_int(args[0])
	if e != nil {
		return
	}
	return Matches{{n, false, nil}}
}
func (n *IntValue) Exec() { n.exec(n.val) }

type StrValue Value[string]

func (n *StrValue) Match(args []string) (matches Matches) {
	if len(args) != 1 || args[0] == "" {
		return
	}
	n.val = args[0]
	return Matches{{n, false, nil}}
}
func (n *StrValue) Exec() { n.exec(n.val) }

type File struct {
	fn   string // executing target
	ext  string // target extension ".json"
	exec func(fn string)
}

func (n *File) Match(args []string) (matches Matches) {
	if len(args) != 1 || args[0] == "" {
		return
	}
	dir, inputFn := filepath.Split(args[0])
	if dir == "" {
		dir = "."
	}
	files, e := ioutil.ReadDir(dir)
	if e != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() { // only match file
			continue
		}
		fn := f.Name()
		ext := filepath.Ext(fn)
		if ext != n.ext { // match extension
			continue
		}

		// fmt.Println(fn, ", ", args[0])

		if fn == inputFn { // fully match
			n.fn = fn
			matches = append(matches, Match{n, false, nil})
		} else if strings.Contains(fn, inputFn) { // partially match
			matches = append(matches, Match{n, true, &prompt.Suggest{fn, ""}})
		}
	}

	return
}
func (n *File) Exec() { n.exec(n.fn) }
