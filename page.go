package main

import (
	"fmt"

	"github.com/ghetzel/go-stockutil/log"
)

type Page struct {
	Name     string          `yaml:"-"`
	Buttons  map[int]*Button `yaml:"buttons"`
	Defaults *Button         `yaml:"defaults"`
	Helper   string          `yaml:"helper"`
	deck     *Deck
}

func (self *Page) Render() error {
	var merr error

	for i, btn := range self.Buttons {
		btn.page = self
		btn.Index = i
		log.AppendError(merr, btn.Render())
	}

	return merr
}

func (self *Page) Sync() error {
	if self.deck == nil {
		return fmt.Errorf("cannot sync page: no deck specified")
	} else if len(self.Buttons) == 0 {
		self.Buttons = make(map[int]*Button)
	}

	for i := 1; i <= self.deck.Count; i++ {
		if _, ok := self.Buttons[i]; !ok {
			self.Buttons[i] = NewButton(self, i)
			self.Buttons[i].auto = true
		}

		self.Buttons[i].page = self
		self.Buttons[i].Index = i

		go self.Buttons[i].Sync()
	}

	return nil
}

func (self *Page) trigger(i int) error {
	if btn, ok := self.Buttons[i]; ok {
		return btn.Trigger()
	} else {
		return nil
	}
}
