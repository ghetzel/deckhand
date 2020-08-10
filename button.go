package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghetzel/diecast"
	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/colorutil"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
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
	Fill            string  `default:"#000000"`
	Color           string  `default:"#FFFFFF"`
	FontName        string  `default:"monospace"`
	FontSize        float64 `default:"64"`
	Text            string
	Action          string
	State           string
	auto            bool
	evaluatedText   string
	evaluatedAction string
	evaluatedState  string
	image           image.Image
	page            *Page
	visualArena     *canvas.Canvas
	fontFamily      *canvas.FontFamily
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
		val = colorutil.MustParse(self.Fill).String()
	case `color`:
		val = colorutil.MustParse(self.Color).String()
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

func (self *Button) path(filename ...string) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%02d", self.Index))

	if self.evaluatedState != `` {
		if candidate := self.page.path(append(parts, self.evaluatedState)...); fileutil.DirExists(candidate) {
			parts = append(parts, self.evaluatedState)
		}
	}

	return self.page.path(append(parts, filename...)...)
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

// Uses the existing values that have already been parsed from the various files and evaluates them.
func (self *Button) regen() {
	self.visualArena = canvas.New(72, 72)

	var ctx = canvas.NewContext(self.visualArena)

	if out, err := diecast.EvalInline(self.Action, nil, templateFunctions); err == nil {
		self.evaluatedAction = out
	} else {
		log.Errorf("tpl: action: %v", err)
		self.evaluatedAction = ``
	}

	if c, err := colorutil.Parse(self.Fill); err == nil {
		ctx.SetFillColor(c.NativeRGBA())
		ctx.SetStrokeColor(canvas.Transparent)
		ctx.DrawPath(
			0,
			0,
			canvas.RoundedRectangle(self.visualArena.W, self.visualArena.H, self.visualArena.H*0.2),
		)
	}

	if out, err := diecast.EvalInline(self.Text, nil, templateFunctions); err == nil {
		self.evaluatedText = out
	} else {
		log.Errorf("tpl: text: %v", err)
		self.evaluatedText = ``
	}

	if img := self.image; img != nil {
		ctx.DrawImage(0, 0, img, 1)
	}

	if self.fontFamily == nil && self.FontName != `` {
		var font = canvas.NewFontFamily(`text`)

		if fileutil.FileExists(self.FontName) {
			if err := font.LoadFontFile(self.FontName, canvas.FontRegular); err == nil {
				self.fontFamily = font
			}
		} else if err := font.LoadLocalFont(self.FontName, canvas.FontRegular); err == nil {
			self.fontFamily = font
		}
	}

	if self.fontFamily != nil {
		if c, err := colorutil.Parse(self.Color); err == nil {
			var face = self.fontFamily.Face(self.FontSize, c.NativeRGBA(), canvas.FontRegular, canvas.FontNormal)
			var text = canvas.NewTextBox(face, self.evaluatedText, ctx.Width(), ctx.Height(), canvas.Center, canvas.Center, 0, 0)

			ctx.DrawText(0, ctx.Height(), text)
		}
	}
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

func (self *Button) syncState() {
	if data, err := fileutil.ReadFirstLine(self.path(`state`)); err == nil {
		self.State = strings.TrimSpace(data)
	} else {
		self.State = ``
	}

	if out, err := diecast.EvalInline(self.State, nil, templateFunctions); err == nil {
		out = strings.TrimSpace(out)

		self.evaluatedState = out
		log.Debugf("button %02d: state=%v", self.Index, self.evaluatedState)
	} else {
		log.Errorf("tpl: state: %v", err)
		self.evaluatedState = ``
	}
}

func (self *Button) Sync() error {
	self.syncState()

	defer func() {
		defaults.SetDefaults(self)

		log.Debugf("  % 8s: %s", `state`, self.State)
		log.Debugf("  % 8s: %s", `text`, self.Text)
		log.Debugf("  % 8s: %s", `action`, self.Action)
		log.Debugf("  % 8s: %s", `fill`, self.Fill)
		log.Debugf("  % 8s: %s", `color`, self.Color)
		log.Debugf("  % 8s: %v", `image`, self.image)
		log.Debugf("  % 8s: %s (%vpt)", `font`, self.FontName, self.FontSize)

		self.regen()
	}()

	var px = filepath.Join(self.path(`*`))

	if matches, err := filepath.Glob(px); err == nil {
		self.image = nil
		self.Color = ``
		self.Fill = ``
		self.Text = ``
		self.Action = ``
		self.FontName = ``
		self.FontSize = 0
		defaults.SetDefaults(self)

		log.Debugf("button %02d [%s]:", self.Index, self.path())

		for _, b := range matches {
			if !fileutil.IsNonemptyFile(b) {
				continue
			}

			var base = strings.ToLower(filepath.Base(b))
			var _, prop = stringutil.SplitPairTrailing(base, `_`)

			switch prop {
			case `color`:

				if data, err := fileutil.ReadFirstLine(b); err == nil {
					if v := strings.TrimSpace(data); v != `` {
						self.Color = v
					}
				}

			case `fill`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					if v := strings.TrimSpace(data); v != `` {
						self.Fill = v
					}
				}

			case `text`:
				if data, err := fileutil.ReadAllString(b); err == nil {
					self.Text = strings.TrimSpace(data)
				}

			case `action`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					self.Action = strings.TrimSpace(data)
				}

			case `font`:
				if data, err := fileutil.ReadFirstLine(b); err == nil {
					for i, val := range strings.Split(data, `:`) {
						val = strings.TrimSpace(val)

						if val != `` {
							switch i {
							case 0:
								self.FontName = val
								self.fontFamily = nil
							case 1:
								if sz := typeutil.Float(val); sz > 0 {
									self.FontSize = sz
								}
							}
						}
					}
				}

			case `image`:
				if err := self.SetImage(b); err != nil {
					return err
				}

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
