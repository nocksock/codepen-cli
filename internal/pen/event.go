package pen

import "github.com/nocksock/codepen-cli/internal/codepen"

// Reporter receives a Mirror's progress.
type Reporter interface {
	Report(Event)
}

// Event is one step of a push or pull: what was planned, built or written, then a summary.
type Event interface {
	event()
}

// Planned lists what a push is about to send; New marks the files the Pen lacks.
type Planned struct {
	Changes codepen.ChangeSet
	New     map[string]bool
}

// Built is a settled build, with its patches and errors.
type Built struct{ Build codepen.Build }

// Downloaded is a remote file written locally.
type Downloaded struct{ Path string }

// Kept is a local edit a pull left alone, because the Pen's file is unchanged.
type Kept struct{ Path string }

// Created is a Pen made by the first push.
type Created struct {
	Title string
	URLs  codepen.URLs
}

// Connected is a directory that now mirrors another Pen.
type Connected struct {
	Title, ID string
	URLs      codepen.URLs
}

// Pushed ends a push that sent Files changed files.
type Pushed struct {
	Title string
	Files int
}

// Pulled ends a pull that wrote Files files.
type Pulled struct{ Files int }

// WouldPush ends a dry run that would send Files changed files.
type WouldPush struct {
	Title string
	Files int
}

// WouldCreate ends a dry run that would create a Pen with Files files; with
// none, from CodePen's starter files.
type WouldCreate struct{ Files int }

// UpToDate ends a push or pull that had nothing to do.
type UpToDate struct{}

func (Planned) event()     {}
func (Built) event()       {}
func (Downloaded) event()  {}
func (Kept) event()        {}
func (Created) event()     {}
func (Connected) event()   {}
func (Pushed) event()      {}
func (Pulled) event()      {}
func (UpToDate) event()    {}
func (WouldPush) event()   {}
func (WouldCreate) event() {}
