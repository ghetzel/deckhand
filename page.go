package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
)

type Page struct {
	Name       string          `yaml:"-"`
	Buttons    map[int]*Button `yaml:"buttons"`
	Defaults   *Button         `yaml:"defaults"`
	Helper     string          `yaml:"helper"`
	HelperArgs string          `yaml:"helperArgs"`
	deck       *Deck
	everHelped bool
	everSynced bool
}

func init() {
	maputil.UnmarshalStructTag = `yaml`
}

func (self *Page) Render() error {
	var merr error

	if !self.everHelped {
		if err := self.RunHelper(); err != nil {
			log.Warningf("helper %v: %v", self.Helper, err)
		}

		self.everHelped = true
	}

	if !self.everSynced {
		if err := self.Sync(); err != nil {
			return err
		}

		self.everSynced = true
	}

	for i, btn := range self.Buttons {
		btn.page = self
		btn.Index = i
		log.AppendError(merr, btn.Render())
	}

	return merr
}

func (self *Page) RunHelper() error {
	if self.Helper != `` {
		if helper, ok := self.deck.Helpers[self.Helper]; ok && helper != `` {
			var helperTempPattern = fmt.Sprintf("deckhand-%s-%s-", self.deck.Name, self.Name)
			var start = time.Now()

			if tmp, err := fileutil.WriteTempFile(helper, helperTempPattern); err == nil {
				os.Chmod(tmp, 0700)
				defer os.Remove(tmp)

				if btn, err := executil.ShellCommand(
					strings.TrimSpace(tmp + ` ` + self.HelperArgs),
				).Output(); err == nil {
					log.Debugf("helper %v: took %v", self.Helper, time.Since(start))

					for _, line := range strings.Split(string(btn), "\n") {
						var preserveExisting bool

						line = strings.TrimSpace(line)

						if line == `` || strings.HasPrefix(line, `#`) {
							continue
						} else if strings.HasPrefix(line, `@`) {
							var atDirective = strings.TrimPrefix(line, `@`)
							atDirective = strings.ToLower(atDirective)

							switch atDirective {
							case `clear`:
								self.deck.Clear()
								self.Buttons = make(map[int]*Button)
							case `preserve`:
								preserveExisting = true
							}

							continue
						}

						if k, v := stringutil.SplitPair(line, `=`); k != `` {
							var bkey = strings.Split(k, `.`)
							var bidx int = int(typeutil.Int(bkey[0]))
							var btn *Button
							var wasThere bool

							if b, ok := self.Buttons[bidx]; ok {
								btn = b
								wasThere = true
							} else {
								btn = NewButton(self, bidx)
								self.Buttons[bidx] = btn
								self.Buttons[bidx].auto = true
							}

							btn.page = self

							if wasThere && preserveExisting {
								continue
							}

							if btn != nil {
								maputil.DeepSet(btn, bkey[1:], typeutil.Auto(v))
								// log.Debugf("set %v -> %v", bkey[1:], v)
								// log.Debugf("    %v <- %v", maputil.DeepGet(btn, bkey[1:]), bkey[1:])
							}
						}
					}
				} else {
					return err
				}
			} else {
				return err
			}
		}
	}

	return nil
}

func (self *Page) Sync() error {
	if self.deck == nil {
		return fmt.Errorf("cannot sync page: no deck specified")
	} else if len(self.Buttons) == 0 {
		self.Buttons = make(map[int]*Button)
	}

	if err := self.RunHelper(); err != nil {
		log.Warningf("helper %v: %v", self.Helper, err)
	}

	self.dump()

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

func (self *Page) dump() {
	for kv := range maputil.M(self.Buttons).Iter(maputil.IterOptions{
		SortKeys: true,
	}) {
		if btn, ok := kv.Value.(*Button); ok {
			log.Debugf(
				"page[%v,%02s]: fill=%v color=%v font=%v:%v state=%v text=%v",
				self.Name,
				kv.Key,
				btn._property(`fill`),
				btn._property(`color`),
				btn._property(`fontName`),
				btn._property(`fontSize`),
				btn._property(`state`),
				btn._property(`text`),
			)
		}
	}
}
