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

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/colorutil"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/rasterizer"
)

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

	fm[`shellOK`] = func(cmdline string) bool {
		var cmd = executil.ShellCommand(cmdline)
		cmd.InheritEnv = true

		return (cmd.Run() == nil)
	}

	return fm
}()

type Button struct {
	Index           int
	Fill            string             `yaml:"fill"     default:"#000000"`
	Color           string             `yaml:"color"    default:"#FFFFFF"`
	FontName        string             `yaml:"fontName" default:"monospace"`
	FontSize        float64            `yaml:"fontSize" default:"64"`
	Text            string             `yaml:"text"`
	Action          string             `yaml:"action"`
	State           string             `yaml:"state"`
	States          map[string]*Button `yaml:"states"`
	auto            bool
	evaluatedText   string
	evaluatedAction string
	evaluatedState  string
	image           image.Image
	page            *Page
	visualArena     *canvas.Canvas
	fontFamily      *canvas.FontFamily
	hasChanges      bool
}

func NewButton(page *Page, i int) *Button {
	var btn = &Button{
		page:  page,
		Index: i,
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
		val = self._fill().String()
	case `color`:
		val = self._color().String()
	case `text`:
		val = self.evaluatedText
	case `action`:
		val = self.evaluatedAction
	case `state`:
		val = self.evaluatedState
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

func (self *Button) _action() string {
	var v string

	if self.Action != `` {
		v = self.Action
	} else if inherit := self.page.Defaults; inherit != nil {
		v = inherit.Action
	}

	if out, err := diecast.EvalInline(v, nil, templateFunctions); err == nil {
		return out
	} else {
		log.Errorf("tpl: action: %v", err)
		return self.evaluatedAction
	}
}

func (self *Button) _text() string {
	var v string

	if self.Text != `` {
		v = self.Text
	} else if inherit := self.page.Defaults; inherit != nil {
		v = inherit.Text
	}

	if out, err := diecast.EvalInline(v, nil, templateFunctions); err == nil {
		return out
	} else {
		log.Errorf("tpl: text: %v", err)
		return self.evaluatedText
	}
}

func (self *Button) _state() string {
	var v string

	if self.State != `` {
		v = self.State
	} else if inherit := self.page.Defaults; inherit != nil {
		v = inherit.State
	}

	if out, err := diecast.EvalInline(v, nil, templateFunctions); err == nil {
		return out
	} else {
		log.Errorf("tpl: state: %v", err)
		return self.evaluatedState
	}
}

func (self *Button) _fill() colorutil.Color {
	var v string

	if self.Fill != `` {
		v = self.Fill
	} else if inherit := self.page.Defaults; inherit != nil {
		v = inherit.Fill
	}

	if c, err := colorutil.Parse(v); err == nil {
		return c
	} else {
		return colorutil.MustParse(`#000000`)
	}
}

func (self *Button) _color() colorutil.Color {
	var v string

	if self.Color != `` {
		v = self.Color
	} else if inherit := self.page.Defaults; inherit != nil {
		v = inherit.Color
	}

	if c, err := colorutil.Parse(v); err == nil {
		return c
	} else {
		return colorutil.MustParse(`#FFFFFF`)
	}
}

func (self *Button) _fontName() string {
	if self.FontName != `` {
		return self.FontName
	} else if inherit := self.page.Defaults; inherit != nil && inherit.FontName != `` {
		return inherit.FontName
	} else {
		return `monospace`
	}
}

func (self *Button) _fontSize() float64 {
	if self.FontSize > 0 {
		return self.FontSize
	} else if inherit := self.page.Defaults; inherit != nil && inherit.FontSize > 0 {
		return inherit.FontSize
	} else {
		return 64
	}
}

// Uses the existing values that have already been parsed from the various files and evaluates them.
func (self *Button) regen() {
	self.visualArena = canvas.New(72, 72)

	if v := self._action(); v != self.evaluatedAction || self.evaluatedAction == `` {
		self.evaluatedAction = v
		self.hasChanges = true
	}

	if v := self._text(); v != self.evaluatedText || self.evaluatedText == `` {
		self.evaluatedText = v
		self.hasChanges = true
	}

	if v := self._state(); v != self.evaluatedState || self.evaluatedState == `` {
		self.evaluatedState = v
		self.hasChanges = true
	}

	// if !self.hasChanges {
	// 	return
	// }

	var ctx = canvas.NewContext(self.visualArena)

	ctx.SetFillColor(self._fill().NativeRGBA())
	ctx.SetStrokeColor(canvas.Transparent)
	ctx.DrawPath(
		0,
		0,
		canvas.RoundedRectangle(self.visualArena.W, self.visualArena.H, self.visualArena.H*0.2),
	)

	if img := self.image; img != nil {
		ctx.DrawImage(0, 0, img, 1)
	}

	if fontName := self._fontName(); self.fontFamily == nil && fontName != `` {
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
			self._fontSize(),
			self._color().NativeRGBA(),
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

	if rendered := rasterizer.Draw(self.visualArena, 1); rendered != nil {
		return png.Encode(w, rendered)
	}

	return nil
}

func (self *Button) Render() error {
	self.regen()

	if rendered := rasterizer.Draw(self.visualArena, 1); rendered != nil {
		if err := self.page.deck.device.WriteRawImageToButton(self.Index, rendered); err != nil {
			return err
		}
	}

	return nil
}

func (self *Button) Sync() error {
	defaults.SetDefaults(self)
	self.hasChanges = true
	self.regen()

	log.Debugf("%02d| % 6s: %s", self.Index, `state`, self.evaluatedState)
	log.Debugf("%02d| % 6s: %s", self.Index, `text`, self.evaluatedText)
	log.Debugf("%02d| % 6s: %s", self.Index, `action`, self.evaluatedAction)
	log.Debugf("%02d| % 6s: %v", self.Index, `fill`, self._fill())
	log.Debugf("%02d| % 6s: %v", self.Index, `color`, self._color())
	log.Debugf("%02d| % 6s: %s (%vpt)", self.Index, `font`, self._fontName(), self._fontSize())
	// log.Debugf("%02d| % 6s: %v", self.Index, `image`, self.image)

	return nil
}

func (self *Button) Trigger() error {
	if !self.isReady() {
		return nil
	}

	defer self.Sync()

	if self.evaluatedAction != `` {
		var action, arg = stringutil.SplitPair(self.evaluatedAction, `:`)

		action = strings.ToLower(action)

		log.Debugf("button %02d: trigger action=%s", self.Index, action)

		switch action {
		case `shell`:
			if arg != `` {
				var cmd = executil.ShellCommand(arg)
				cmd.InheritEnv = true

				return cmd.Run()
			} else {
				return fmt.Errorf("Action 'shell' must be given an argument")
			}
		case `page`:
			self.page.deck.Page = arg
			return self.page.deck.Sync()
		}
	}

	return nil
}
