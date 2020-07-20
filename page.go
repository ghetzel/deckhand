package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/typeutil"
)

type Page struct {
	Name    string
	Buttons map[int]*Button
	deck    *Deck
}

func (self *Page) path(filename ...string) string {
	return self.deck.path(append([]string{self.Name}, filename...)...)
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

	if fileutil.FileExists(self.path(DeckhandLockFile)) {
		return nil
	}

	if btndirs, err := ioutil.ReadDir(self.path()); err == nil {
		var buttons = make(map[int]*Button)

		for _, btndir := range btndirs {
			if bidx := strings.TrimPrefix(btndir.Name(), `0`); typeutil.IsNumeric(bidx) {
				var bi = int(typeutil.Int(bidx))

				if c := (bi + 1); c > self.deck.Count {
					self.deck.Count = c
				}

				buttons[bi] = &Button{
					Index: bi,
					page:  self,
				}
			}
		}

		self.Buttons = buttons
		var merr error

		for i := 0; i < self.deck.Count; i++ {
			if btn, ok := self.Buttons[i]; ok {
				btn.Sync()
			} else {
				self.Buttons[i] = NewButton(self, i)
				self.Buttons[i].auto = true
				self.Buttons[i].Render()
			}
		}

		return merr
	} else {
		return err
	}
}

func (self *Page) trigger(i int) error {
	if btn, ok := self.Buttons[i]; ok {
		return btn.Trigger()
	} else {
		return nil
	}
}
