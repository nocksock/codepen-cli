package pen

import "github.com/nocksock/codepen-cli/internal/codepen"

// Reporter receives a Mirror's progress.
type Reporter interface {
	Report(Event)
}

// Event is one step of a push or pull: Planned, Built, Downloaded, Created or Connected.
type Event interface {
	event()
}

// Planned lists what a push is about to send.
type Planned struct{ Changes codepen.ChangeSet }

// Built is a settled build, with its patches and errors.
type Built struct{ Build codepen.Build }

// Downloaded is a remote file written locally.
type Downloaded struct{ Path string }

// Created is a Pen made by the first push.
type Created struct{ URLs codepen.URLs }

// Connected is a directory that now mirrors another Pen.
type Connected struct{ URLs codepen.URLs }

func (Planned) event()    {}
func (Built) event()      {}
func (Downloaded) event() {}
func (Created) event()    {}
func (Connected) event()  {}
