package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-stockutil/colorutil"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/rxutil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
	"github.com/mcuadros/go-defaults"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// Specifies a symbolic mapping between text lines in the per-button
// configuration and what text is output in the final render.
var EntityMap = func() (m sync.Map) {
	m.Store(`---`, strings.Repeat("\u2500", 10))
	m.Store(`-!-`, strings.Repeat("\u2501", 10))
	m.Store(`|||`, strings.Repeat("\u2509", 10))
	m.Store(`===`, strings.Repeat("\u2550", 10))
	m.Store(`...`, strings.Repeat("\u2504", 10))
	m.Store(`.!.`, strings.Repeat("\u2505", 10))

	return
}()

const MultiActionSeparator = `->`

var templateFunctions = func() diecast.FuncMap {
	var fm = diecast.GetStandardFunctions(nil)

	fm[`shell`] = func(cmdline string) (string, error) {
		var cmd = executil.ShellCommand(cmdline)
		cmd.InheritEnv = true

		if out, err := cmd.Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		} else {
			return ``, nil
		}
	}

	fm[`shellJson`] = func(cmdline string) (interface{}, error) {
		var cmd = executil.ShellCommand(cmdline)
		cmd.InheritEnv = true

		if out, err := cmd.Output(); err == nil {
			var oj interface{}

			if err := json.Unmarshal([]byte(out), &oj); err == nil {
				return oj, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	fm[`shellOK`] = func(cmdline string) bool {
		var cmd = executil.ShellCommand(cmdline)
		cmd.InheritEnv = true

		return (cmd.Run() == nil)
	}

	// fm[`cpuperc`] = func(metric string) float64 {}

	return fm
}()

type Button struct {
	Index         int
	Fill          string             `yaml:"fill"          default:"#000000"`
	Color         string             `yaml:"color"         default:"#FFFFFF"`
	FontName      string             `yaml:"fontName"      default:"monospace"`
	FontSize      float64            `yaml:"fontSize"      default:"64"`
	Text          string             `yaml:"text"`
	Icon          string             `yaml:"icon"`
	Progress      string             `yaml:"progress"`
	ProgressColor string             `yaml:"progressColor" default:"#FFFFFF"`
	Maximum       string             `yaml:"maximum"`
	Action        string             `yaml:"action"`
	State         string             `yaml:"state"`
	States        map[string]*Button `yaml:"states"`
	Layers        []*Button          `yaml:"layers"`
	// Visible           string             `yaml:"visible"`
	auto              bool
	sticky            bool
	override          *Button
	evaluatedText     string
	evaluatedIcon     string
	evaluatedAction   string
	overrideState     string
	evaluatedState    string
	evaluatedProgress float64
	evaluatedMaximum  float64
	evaluatedFontName string
	evaluatedColor    string
	evaluatedFill     string
	evaluatedFontSize float64
	image             image.Image
	page              *Page
	visualArena       *canvas.Canvas
	fontFamily        *canvas.FontFamily
	hasChanges        bool
}

func NewButton(page *Page, i int) *Button {
	var btn = &Button{
		Index: i,
		page:  page,
	}

	return btn
}

func (self *Button) MarshalJSON() ([]byte, error) {
	type Alias Button

	return json.Marshal(&struct {
		*Alias
		Image string
	}{
		Alias: (*Alias)(self),
		Image: fmt.Sprintf(
			"/deckhand/v1/decks/%s/%s/%d/image/?state=%s",
			self.page.deck.Name,
			self.page.Name,
			self.Index,
			self.evaluatedState,
		),
	})
}

func (self *Button) ServeProperty(w http.ResponseWriter, req *http.Request, propname string) {
	var val string

	switch propname {
	case `fill`:
		val = self._property(`Fill`).String()
	case `color`:
		val = self._property(`Color`).String()
	case `text`:
		val = self.evaluatedText
	case `action`:
		val = self.evaluatedAction
	case `icon`:
		val = self.evaluatedIcon
	case `state`:
		val = self.evaluatedState
	case `visible`:
		val = typeutil.String(self._property(`Visible`).Bool())
	case `image`:
		if self.image != nil {
			w.Header().Set(`Content-Type`, `image/png`)
			png.Encode(w, self.image)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

		return
	}

	w.Header().Set(`Content-Type`, `text/plain`)
	w.Write([]byte(val))
}

func (self *Button) Reset() {
	self.Color = ``
	self.Fill = ``
	self.Text = ``
	self.Action = ``
	self.State = ``
	self.States = nil
	self.FontName = ``
	self.FontSize = 0
	defaults.SetDefaults(self)
}

func (self *Button) SetProperty(propname string, value interface{}) {
	switch propname {
	case `fill`:
		self.Fill = typeutil.String(value)
	case `color`:
		self.Color = typeutil.String(value)
	case `text`:
		self.Text = typeutil.String(value)
	case `action`:
		self.Action = typeutil.String(value)
	case `icon`:
		self.Icon = typeutil.String(value)
	case `state`:
		self.State = typeutil.String(value)
	case `fontSize`:
		self.FontSize = typeutil.Float(value)
	case `fontName`:
		self.FontName = typeutil.String(value)
	// case `visible`:
	// 	self.Visible = typeutil.String(value)
	default:
		maputil.M(self).Set(propname, value)
	}
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

func (self *Button) _property(name string) typeutil.Variant {
	var value = typeutil.V(nil)
	var strct = maputil.M(self)
	var ovrid = maputil.M(self.override)

	if name != `State` {
		if ov := self.overrideState; ov != `` {
			self.evaluatedState = ov
		}

		if state, ok := self.States[self.evaluatedState]; ok && state != nil {
			if stateSpecificValue := state._property(name); !stateSpecificValue.IsNil() {
				return stateSpecificValue
			}
		}
	}

	if v := ovrid.Get(name); !v.IsZero() {
		value = v
	} else if v := strct.Get(name); !v.IsZero() {
		value = v
	} else if self.page != nil {
		if inherit := self.page.Defaults; inherit != nil {
			return inherit._property(name)
		}
	}

	if vS := value.String(); strings.Contains(vS, `{{`) && strings.Contains(vS, `}}`) {
		if self.page != nil {
			if out, err := self.page.eval(value.String()); err == nil {
				return out
			} else {
				log.Warningf("property %s: bad template: %v", name, err)
			}
		}

		return maputil.M(self).Get(`evaluated` + name)
	} else {
		return value
	}
}

// Uses the existing values that have already been parsed from the various files and evaluates them.
func (self *Button) regen() {
	self.visualArena = canvas.New(72, 72)

	// if visible := self._property(`Visible`); visible.String() != `` && !visible.Bool() {
	// 	self.Reset()
	// 	return
	// }

	if v := self._property(`State`).String(); v != self.evaluatedState || self.evaluatedState == `` {
		self.evaluatedState = v
		self.hasChanges = true
	}

	if v := self._property(`Icon`).String(); v != `` {
		if ico, ok := self.page.deck.Icons[v]; ok {
			var i = ico
			self.override = &i
		} else {
			self.override = nil
		}
	}

	if v := self._property(`Action`).String(); v != self.evaluatedAction || self.evaluatedAction == `` {
		self.evaluatedAction = v
		self.hasChanges = true
	}

	if v := self._property(`Progress`).Float(); v != self.evaluatedProgress || self.evaluatedProgress == 0 {
		self.evaluatedProgress = v
		self.hasChanges = true
	}

	if v := self._property(`Maximum`).Float(); v != self.evaluatedMaximum || self.evaluatedMaximum == 0 {
		self.evaluatedMaximum = v
		self.hasChanges = true
	}

	if v := self._property(`Text`).String(); v != self.evaluatedText || self.evaluatedText == `` {
		var lines = strings.Split(v, "\n")

		for i, line := range lines {
			if repl := maputil.M(&EntityMap).String(line); repl != `` {
				lines[i] = repl
			}
		}

		self.evaluatedText = strings.Join(lines, "\n")
		self.hasChanges = true
	}

	if !self.hasChanges {
		return
	}

	var ctx = canvas.NewContext(self.visualArena)

	ctx.SetFillColor(colorutil.MustParse(self._property(`Fill`).String()).NativeRGBA())
	ctx.SetStrokeColor(canvas.Transparent)
	ctx.DrawPath(
		0,
		0,
		canvas.RoundedRectangle(
			self.visualArena.W,
			self.visualArena.H,
			self.visualArena.H*0.2,
		),
	)

	if img := self.image; img != nil {
		ctx.DrawImage(0, 0, img, 1)
	}

	if fontName := self._property(`FontName`).String(); self.fontFamily == nil && fontName != `` {
		var font = canvas.NewFontFamily(`text`)

		if fileutil.FileExists(fontName) {
			if err := font.LoadFontFile(fontName, canvas.FontRegular); err == nil {
				self.fontFamily = font
			}
		} else if err := font.LoadLocalFont(fontName, canvas.FontRegular); err == nil {
			self.fontFamily = font
		}
	}

	if self.fontFamily != nil {
		var face = self.fontFamily.Face(
			self._property(`FontSize`).Float(),
			colorutil.MustParse(self._property(`Color`).String()).NativeRGBA(),
			canvas.FontRegular,
			canvas.FontNormal,
		)

		var text = canvas.NewTextBox(
			face,
			self.evaluatedText,
			ctx.Width(),
			ctx.Height(),
			canvas.Center,
			canvas.Center,
			0,
			0,
		)

		ctx.DrawText(0, ctx.Height(), text)
	}

	// if maximum := self.evaluatedMaximum; maximum > 0 {
	// 	ctx.SetFillColor(canvas.Transparent)
	// 	ctx.SetStrokeColor(colorutil.MustParse(self._property(`ProgressColor`).String()).NativeRGBA())
	// 	ctx.DrawPath(
	// 		0,
	// 		0,
	// 		canvas.RoundedRectangle(self.visualArena.W, self.visualArena.H, self.visualArena.H*0.2),
	// 	)
	// }

	self.hasChanges = false
}

func (self *Button) SetImage(filename string) error {
	if !self.isReady() {
		return nil
	}

	self.image = nil

	if f, err := os.Open(filename); err == nil {
		img, _, err := image.Decode(f)
		f.Close()

		if err == nil {
			self.image = img
			return nil
		} else {
			return err
		}
	} else {
		return err
	}
}

func (self *Button) RenderTo(w io.Writer) error {
	self.regen()

	if rendered := rasterizer.Draw(
		self.visualArena,
		canvas.DPI(72),
		canvas.DefaultColorSpace,
	); rendered != nil {
		return png.Encode(w, rendered)
	}

	return nil
}

func (self *Button) Render() error {
	if self.page == nil {
		return nil
	}

	self.regen()

	if rendered := rasterizer.Draw(
		self.visualArena,
		canvas.DPI(72),
		canvas.DefaultColorSpace,
	); rendered != nil {
		if err := self.page.deck.device.WriteRawImageToButton(self.Index-1, rendered); err != nil {
			return err
		}
	}

	return nil
}

func (self *Button) Sync() error {
	defaults.SetDefaults(self)
	self.hasChanges = true
	self.regen()

	// log.Debugf("%02d| % 6s: %s", self.Index, `state`, self.evaluatedState)
	// log.Debugf("%02d| % 6s: %s", self.Index, `text`, self.evaluatedText)
	// log.Debugf("%02d| % 6s: %s", self.Index, `action`, self.evaluatedAction)
	// log.Debugf("%02d| % 6s: %v", self.Index, `fill`, self._fill())
	// log.Debugf("%02d| % 6s: %v", self.Index, `color`, self._color())
	// log.Debugf("%02d| % 6s: %s (%vpt)", self.Index, `font`, self._fontName(), self._fontSize())
	// log.Debugf("%02d| % 6s: %v", self.Index, `image`, self.image)

	return nil
}

func (self *Button) Trigger() error {
	if !self.isReady() {
		return nil
	}

	defer self.Sync()

	if self.evaluatedAction != `` {
		for _, actionPair := range strings.Split(self.evaluatedAction, MultiActionSeparator) {
			actionPair = strings.TrimSpace(actionPair)

			var action, arg = stringutil.SplitPair(actionPair, `:`)
			var terr error

			action = strings.ToLower(action)

			log.Debugf("button %02d: trigger action=%s state=%s", self.Index, action, self.evaluatedState)

			switch action {
			case `shell`:
				if arg != `` {
					var cmd = executil.ShellCommand(arg)
					cmd.InheritEnv = true

					terr = cmd.Run()
				} else {
					terr = fmt.Errorf("Action 'shell' must be given an argument")
				}
			case `page`:
				var pg, rest = stringutil.SplitPairTrimSpace(arg, `;`)

				self.page.deck.Page = pg
				terr = self.page.deck.Sync()

				if pg := self.page.deck.CurrentPage(); pg != nil {
					pg.setDataFromArgLine(rest, autotypePageData)
				}
			case `http`:
				var httpargs = rxutil.Split(`\s+`, arg)

				if len(httpargs) < 2 {
					terr = fmt.Errorf("usage: http:method url")
				} else {
					var method string = strings.ToUpper(httpargs[0])

					if client, err := httputil.NewClient(httpargs[1]); err == nil {
						if _, err := client.Request(
							httputil.Method(method),
							``,
							nil,
							nil,
							nil,
						); err != nil {
							terr = err
						}
					} else {
						terr = err
					}
				}

			case `state`:
				self.overrideState = arg
				terr = self.Sync()

			case `cleardata`:
				self.page.data = maputil.M(nil)

			case `set`:
				self.page.setDataFromArgLine(arg, autotypePageData)

				for _, pair := range strings.Split(arg, `;`) {
					var k, v = stringutil.SplitPairTrimSpace(pair, `=`)

					self.page.data.Set(k, typeutil.Auto(v))
				}

			case `increment`:
				self.page.setDataFromArgLine(arg, func(m *maputil.Map, kk string, vv string) interface{} {
					var v0, v1 = stringutil.SplitPair(vv, `,`)

					var vi = typeutil.Int(v0)

					if vi == 0 {
						vi = 1
					}

					var vlim = typeutil.Int(v1)
					var next = m.Int(kk) + vi

					if vlim > 0 && next > vlim {
						return vlim
					} else {
						return next
					}
				})

			case `decrement`:
				self.page.setDataFromArgLine(arg, func(m *maputil.Map, kk string, vv string) interface{} {
					var v0, v1 = stringutil.SplitPair(vv, `,`)
					var vi = typeutil.Int(v0)
					var vlim = typeutil.Int(v1)
					var vnext = m.Int(kk) - vi

					log.Debugf("decr: key=%s, v0=%v v1=%v vi=%v vlim=%v next=%v", kk, v0, v1, vi, vlim, vnext)

					if vnext < vlim {
						return vlim
					} else {
						return vnext
					}
				})

			}

			if terr != nil {
				return terr
			}
		}
	}

	return nil
}

func autotypePageData(m *maputil.Map, kk string, vv string) interface{} {
	return typeutil.Auto(vv)
}
