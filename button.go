package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghetzel/go-stockutil/colorutil"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/stringutil"
)

type Button struct {
	Index    int
	Fill     colorutil.Color
	Color    colorutil.Color
	Image    image.Image
	Text     string
	Delegate string
	Action   string
	State    string
	auto     bool
	page     *Page
}

func NewButton(page *Page, i int) *Button {
	return &Button{
		Fill:  colorutil.MustParse(`#000000`),
		Color: colorutil.MustParse(`#FFFFFF`),
		page:  page,
		Index: i,
	}
}

func (self *Button) path(filename ...string) string {
	return self.page.path(append([]string{
		fmt.Sprintf("%02d", self.Index),
	}, filename...)...)
}

func (self *Button) isReady() bool {
	if self.page == nil {
		return false
	}

	if self.page.deck == nil {
		return false
	}

	if self.page.deck.device == nil {
		return false
	}

	return true
}

func (self *Button) SetImage(filename string) error {
	if !self.isReady() {
		return nil
	}

	self.Image = nil

	if f, err := os.Open(filename); err == nil {
		img, _, err := image.Decode(f)
		f.Close()

		if err == nil {
			self.Image = img
			return nil
		} else {
			return err
		}
	} else {
		return err
	}
}

func (self *Button) Render() error {
	if self.Image != nil {
		if err := self.page.deck.device.WriteRawImageToButton(self.Index, self.Image); err != nil {
			return err
		}
	} else if err := self.page.deck.device.WriteColorToButton(self.Index, self.Fill.NativeRGBA()); err != nil {
		return err
	}

	if self.Text != `` {
		self.page.deck.device.WriteTextToButton(self.Index, self.Text, self.Color.NativeRGBA(), color.Transparent)
	}

	return nil
}

func (self *Button) Sync() error {
	var px = filepath.Join(self.path(`*`))

	if matches, err := filepath.Glob(px); err == nil {
		if self.State == `` {
			self.Image = nil
			self.Text = ``
			self.Action = ``
		}

		for _, b := range matches {
			var base = strings.ToLower(filepath.Base(b))
			var state, prop = stringutil.SplitPairTrailing(base, `_`)

			if self.State == `` && state != `` {
				continue
			} else if self.State != `` && state == `` {
				continue
			}

			switch prop {
			case `color`, `fill`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					if c, err := colorutil.Parse(strings.TrimSpace(data)); err == nil {
						if prop == `color` {
							self.Color = c
						} else {
							self.Fill = c
						}
					}
				}

			case `text`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					self.Text = strings.TrimSpace(data)
				}

			case `action`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					self.Action = strings.TrimSpace(data)
				}

			case `image`:
				self.SetImage(b)

			default:
				if fileutil.IsNonemptyExecutableFile(b) {
					self.Action = b
				}
			}
		}

		return nil
	} else {
		return err
	}
}

func (self *Button) Trigger() error {
	if !self.isReady() {
		return nil
	}

	if self.Action != `` {
		var action, arg = stringutil.SplitPair(self.Action, `:`)

		action = strings.ToLower(action)

		switch action {
		case `shell`:
			if arg != `` {
				return executil.ShellCommand(arg).Run()
			} else {
				return fmt.Errorf("Action 'shell' must be given an argument")
			}
		case `state`:
			self.State = arg
			return self.Sync()
		case `page`:
			self.page.deck.Page = arg
			return self.page.deck.Sync()
		}
	}

	return nil
}
