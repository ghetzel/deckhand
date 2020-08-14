package main

import (
	"fmt"

	"github.com/ghetzel/go-stockutil/log"
)

type Page struct {
	Name     string          `yaml:"-"`
	Buttons  map[int]*Button `yaml:"buttons"`
	Defaults *Button         `yaml:"defaults"`
	deck     *Deck
}

func (self *Page) Render() error {
	var merr error

	for _, btn := range self.Buttons {
		log.AppendError(merr, btn.Render())
	}

	return merr
}

func (self *Page) Sync() error {
	if self.deck == nil {
		return fmt.Errorf("cannot sync page: no deck specified")
	}

	var merr error

	for _, btn := range self.Buttons {
		btn.page = self
		log.AppendError(merr, btn.Sync())
	}

	// for i := 0; i < self.deck.Count; i++ {
	// 	if btn, ok := self.Buttons[i]; ok {
	// 		log.AppendError(merr, btn.Sync())
	// 	} else {
	// 		self.Buttons[i] = NewButton(self, i)
	// 		self.Buttons[i].auto = true
	// 		self.Buttons[i].Render()
	// 	}
	// }

	return merr
}

func (self *Page) trigger(i int) error {
	if btn, ok := self.Buttons[i]; ok {
		return btn.Trigger()
	} else {
		return nil
	}
}
