package main

import (
	"fmt"
	"go/constant"
	"go/token"
	"os"
	"strings"
	"time"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/sliceutil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
)

type pageSetDataFunc func(m *maputil.Map, key string, value string) interface{}

type Doable interface {
	Do(*Page) (interface{}, error)
	Key() string
}

type DataConfig struct {
	HTTP    []*HttpConfig    `yaml:"http"`
	Exec    []*ShellConfig   `yaml:"exec"`
	Literal []*LiteralConfig `yaml:"literal"`
}

func (self *DataConfig) Doables() (out []Doable) {
	for _, hc := range self.HTTP {
		var v interface{} = hc

		if d, ok := v.(Doable); ok {
			out = append(out, d)
		}
	}

	for _, sc := range self.Exec {
		var v interface{} = sc

		if d, ok := v.(Doable); ok {
			out = append(out, d)
		}
	}

	return
}

type Page struct {
	Name         string          `yaml:"-"`
	DataConfig   DataConfig      `yaml:"data"`
	Buttons      map[int]*Button `yaml:"buttons"`
	Defaults     *Button         `yaml:"defaults"`
	Helper       string          `yaml:"helper"`
	HelperArgs   string          `yaml:"helperArgs"`
	Refresh      string          `yaml:"refresh"`
	deck         *Deck
	everHelped   bool
	everSynced   bool
	lastSyncedAt time.Time
	data         *maputil.Map
	helpRunning  bool
}

func init() {
	maputil.UnmarshalStructTag = `yaml`
}

func (self *Page) Render() error {
	var merr error

	if !self.everHelped {
		for _, btn := range self.Buttons {
			btn.sticky = true
		}

		if err := self.RunHelper(); err != nil {
			log.Warningf("helper %v: %v", self.Helper, err)
		}

		self.everHelped = true
	}

	if !self.everSynced || self.shouldSync() {
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

func (self *Page) shouldSync() bool {
	if self.lastSyncedAt.IsZero() {
		return true
	} else if refresh := typeutil.Duration(self.Refresh); refresh > 0 {
		if time.Since(self.lastSyncedAt) > refresh {
			return true
		}
	}

	return false
}

func (self *Page) syncData() error {
	if self.data == nil {
		self.data = maputil.M(nil)
	}

	for k, v := range self.deck.DataSources.GetAll() {
		self.data.Set(k, v)
	}

	for i, req := range self.DataConfig.Doables() {
		if out, err := req.Do(self); err == nil {
			if out != nil {
				var key = sliceutil.OrString(req.Key(), fmt.Sprintf("data%d", i))

				self.data.Set(key, out)
			}
		} else {
			return fmt.Errorf("request %d: %v", i, err)
		}
	}

	return nil
}

func (self *Page) RunHelper() error {
	if self.Helper != `` {
		if self.helpRunning {
			return nil
		} else {
			self.helpRunning = true
			defer func() {
				self.helpRunning = false
			}()
		}

		if helper, ok := self.deck.Helpers[self.Helper]; ok && helper != `` {
			if err := self.syncData(); err != nil {
				return err
			}

			var helperTempPattern = fmt.Sprintf("deckhand-%s-%s-", self.deck.Name, self.Name)
			var start = time.Now()

			// write helper data to a file
			if datafile, err := fileutil.WriteTempFile(
				maputil.M(self.dataMap()).JSON(`  `),
				helperTempPattern+`data-`,
			); err == nil {
				os.Chmod(datafile, 0600)
				defer os.Remove(datafile)

				// write helper script to a file
				if tmp, err := fileutil.WriteTempFile(helper, helperTempPattern); err == nil {
					os.Chmod(tmp, 0700)
					defer os.Remove(tmp)

					// var helperArgs []string

					// if self.HelperArgs != `` {
					// 	if args, err := executil.Split(self.HelperArgs); err == nil {
					// 		helperArgs = args
					// 	} else {
					// 		return fmt.Errorf("bad args: %v", err)
					// 	}
					// }

					var helperCmd = executil.ShellCommand(tmp + ` ` + self.HelperArgs)

					self.prepCommand(helperCmd)

					helperCmd.Timeout = time.Second
					helperCmd.Stderr = log.NewWritableLogger(log.WARNING, `helper: `)
					helperCmd.SetEnv(`DIECAST_PAGE_DATA_FILE`, datafile)
					helperCmd.SetEnv(`DECKHAND_DATA_FILE`, datafile)

					if btn, err := helperCmd.Output(); err == nil {
						log.Debugf("helper %v: took %v", self.Helper, time.Since(start))

						for _, line := range strings.Split(string(btn), "\n") {
							var preserveExisting bool

							line = strings.TrimSpace(line)

							if line == `` || strings.HasPrefix(line, `#`) {
								continue
							} else if strings.HasPrefix(line, `@`) {
								var atDirective, rest = stringutil.SplitPair(
									strings.TrimPrefix(line, `@`),
									` `,
								)

								atDirective = strings.ToLower(atDirective)

								switch atDirective {
								case `clear`:
									self.Clear()
								case `preserve`:
									preserveExisting = true
								case `debug`:
									if len(rest) > 0 {
										log.Debugf("HELPER-DEBUG[%s]: %s", self.Helper, rest)
									} else {
										log.Debugf("HELPER-DEBUG[%s]", self.Helper)
									}
								}

								continue
							}

							if k, v := stringutil.SplitPairTrimSpace(line, `=`); k != `` {
								var bkey = strings.Split(k, `.`)
								var bidx int = int(typeutil.Int(bkey[0]))
								var btn *Button
								var wasThere bool

								if b, ok := self.Buttons[bidx]; ok {
									btn = b
									wasThere = true
								} else {
									btn = NewButton(self, bidx)
									btn.auto = true
								}

								defaults.SetDefaults(btn)
								btn.page = self
								self.Buttons[bidx] = btn

								if wasThere && preserveExisting {
									continue
								}

								if btn != nil {
									// this wild nonsense lets us piggypack on golang's own string escaping rules
									v = constant.StringVal(
										constant.MakeFromLiteral(
											`"`+v+`"`,
											token.STRING,
											0,
										),
									)

									btn.SetProperty(
										strings.Join(bkey[1:], `.`),
										typeutil.Auto(v),
									)
								}
							}
						}
					} else {
						return err
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

func (self *Page) Clear() error {
	for i := 1; i <= self.deck.Count; i++ {
		if btn, ok := self.Buttons[i]; ok && btn.sticky {
			continue
		}

		self.Buttons[i] = NewButton(self, i)
		self.Buttons[i].auto = true
		self.Buttons[i].page = self
		self.Buttons[i].Index = i
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

		self.Buttons[i].Sync()
	}

	self.lastSyncedAt = time.Now()
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
				"page[%v,%02s]: fill=%v color=%v font=%v:%v state=%v text=%v data=%+v",
				self.Name,
				kv.Key,
				btn._property(`fill`),
				btn._property(`color`),
				btn._property(`fontName`),
				btn._property(`fontSize`),
				btn._property(`state`),
				btn._property(`text`),
				strings.Join(maputil.StringKeys(self.data), `,`),
			)
		}
	}
}

func (self *Page) eval(dcTemplate string) (typeutil.Variant, error) {
	if out, err := diecast.EvalInline(
		dcTemplate,
		self.dataMap(),
		templateFunctions,
	); err == nil {
		return typeutil.V(out), nil
	} else {
		return typeutil.V(nil), err
	}
}

func (self *Page) prepCommand(cmd *executil.Cmd) {
	cmd.InheritEnv = true

	cmd.SetEnv(`DECKHAND_PAGE`, self.Name)
	cmd.SetEnv(`DECKHAND_DECK`, self.deck.Name)
	cmd.SetEnv(`DECKHAND_DEVICE_BUTTONS`, self.deck.Count)
	cmd.SetEnv(`DECKHAND_DEVICE_ROWS`, self.deck.Rows)
	cmd.SetEnv(`DECKHAND_DEVICE_COLS`, self.deck.Cols)
	cmd.SetEnv(`DECKHAND_DEVICE_MODEL`, self.deck.device.GetName())
}

func (self *Page) setDataFromArgLine(arg string, valfn pageSetDataFunc) {
	if self.data == nil {
		self.data = maputil.M(nil)
	}

	if valfn != nil {
		var arg = strings.TrimSpace(arg)

		for _, pair := range strings.Split(arg, `;`) {
			var k, v = stringutil.SplitPairTrimSpace(pair, `=`)

			if k != `` {
				self.data.Set(k, valfn(self.data, k, v))
				log.Debugf("set %v=%v", k, v)
			}
		}
	}
}

func (self *Page) dataMap() map[string]interface{} {
	if self.data == nil {
		self.data = maputil.M(nil)
	}

	return self.data.MapNative()
}
